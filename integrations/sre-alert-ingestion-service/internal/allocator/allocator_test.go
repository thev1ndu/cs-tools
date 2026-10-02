// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package allocator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"sre-alert-ingestion-service/internal/model"
	"sre-alert-ingestion-service/internal/postgres"
)

type fakeRow struct{ source, alert string }

// fakeStore is an in-memory alert_seq + alerts table with knobs for the failure cases.
type fakeStore struct {
	mu      sync.Mutex
	seq     int64
	rows    map[string]fakeRow
	inserts map[string]int // insert calls per id, filler included

	claims      atomic.Int64 // ClaimRange calls
	insertBatch atomic.Int64 // InsertBatch calls
	// claimErr, if set, is returned by every ClaimRange call.
	claimErr error
	// claimDelay, if set, slows every ClaimRange call so concurrent submissions pile up
	// in the queue before the claimer batches them, the way a real network round trip would.
	claimDelay time.Duration

	// failRealInsert fails inserts of real alerts (not filler rows) for which it returns true.
	failRealInsert func(alert string) bool
	failAllInserts bool
	// failFillers fails this many filler inserts before letting them through.
	failFillers int
	// insertGate, if set, blocks every Insert until closed.
	insertGate chan struct{}

	// claimGate, if set, blocks every ClaimRange until closed; claimEntered is signalled on entry.
	claimGate    chan struct{}
	claimEntered chan struct{}
}

func newFakeStore() *fakeStore {
	return &fakeStore{rows: map[string]fakeRow{}, inserts: map[string]int{}}
}

func (f *fakeStore) ClaimRange(_ context.Context, n int) (int64, error) {
	f.claims.Add(1)
	if f.claimEntered != nil {
		select {
		case f.claimEntered <- struct{}{}:
		default:
		}
	}
	if f.claimGate != nil {
		<-f.claimGate
	}
	if f.claimDelay > 0 {
		time.Sleep(f.claimDelay)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.claimErr != nil {
		return 0, f.claimErr
	}
	start := f.seq + 1
	f.seq += int64(n)
	return start, nil
}

func (f *fakeStore) Insert(_ context.Context, id, source string, alert []byte) error {
	if f.insertGate != nil {
		<-f.insertGate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inserts[id]++
	s := string(alert)
	isFiller := strings.HasPrefix(s, fillerPrefix)
	if isFiller && f.failFillers > 0 {
		f.failFillers--
		return errors.New("write timeout")
	}
	if f.failAllInserts || (!isFiller && f.failRealInsert != nil && f.failRealInsert(s)) {
		return errors.New("write timeout")
	}
	f.rows[id] = fakeRow{source: source, alert: s}
	return nil
}

// InsertBatch simulates pipelining by looping f.Insert per row, preserving only the per-row error-isolation contract InsertBatch promises.
func (f *fakeStore) InsertBatch(ctx context.Context, rows []postgres.InsertRow) []error {
	f.insertBatch.Add(1)
	errs := make([]error, len(rows))
	for i, r := range rows {
		errs[i] = f.Insert(ctx, r.ID, r.Source, r.Alert)
	}
	return errs
}

func (f *fakeStore) InsertFiller(ctx context.Context, id, source, filler string) (bool, string, error) {
	f.mu.Lock()
	row, ok := f.rows[id]
	f.mu.Unlock()
	if ok {
		f.mu.Lock()
		f.inserts[id]++
		f.mu.Unlock()
		return false, row.alert, nil
	}
	return true, "", f.Insert(ctx, id, source, []byte(filler))
}

type recordingNotifier struct {
	mu       sync.Mutex
	failures []StoreFailure
}

func (n *recordingNotifier) StoreFailed(f StoreFailure) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.failures = append(n.failures, f)
}

type countingWaker struct{ n atomic.Int64 }

func (w *countingWaker) Wake() { w.n.Add(1) }

func testConfig() Config {
	return Config{
		QueueSize: 5000, MaxBatch: 200, WriteConcurrency: 64,
		InsertAttempts: 3, InsertBaseDelay: time.Millisecond,
		QueueMaxBytes: 1 << 30, QueryTimeout: time.Millisecond, WriteDeadline: time.Minute,
	}
}

func newTestAllocator(t *testing.T, store *fakeStore, n FailureNotifier, w Waker, cfg Config) *Allocator {
	t.Helper()
	a := New(slog.New(slog.NewTextHandler(io.Discard, nil)), store, n, w, cfg)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.Close(ctx)
	})
	return a
}

func alert(service, uid string) model.Alert {
	return model.Alert{Service: service, MetricName: "HighCPU", Severity: "Critical", Source: "Test", UniqueIdentifier: uid}
}

func seqOf(t *testing.T, id string) int64 {
	t.Helper()
	if !strings.HasPrefix(id, "ALT") || len(id) != 12 {
		t.Fatalf("id %q is not ALT + 9 digits", id)
	}
	n, err := strconv.ParseInt(id[3:], 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestConcurrentSubmits_UniqueConsecutiveIDsWithFewClaims(t *testing.T) {
	store := newFakeStore()
	store.claimDelay = 2 * time.Millisecond // a real claim takes a few ms; lets the queue fill
	a := newTestAllocator(t, store, nil, nil, testConfig())

	const total = 1000
	ids := make([]string, total)
	var wg sync.WaitGroup
	for i := range total {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := a.Submit(context.Background(), "aws", "req", []model.Alert{alert("svc", "u"+strconv.Itoa(i))})
			if err != nil {
				t.Errorf("Submit: %v", err)
				return
			}
			ids[i] = got[0]
		}()
	}
	wg.Wait()

	seen := map[int64]bool{}
	for _, id := range ids {
		n := seqOf(t, id)
		if seen[n] {
			t.Fatalf("id %s issued twice", id)
		}
		seen[n] = true
	}
	for n := int64(1); n <= total; n++ {
		if !seen[n] {
			t.Fatalf("gap: id %d never issued", n)
		}
	}
	if len(store.rows) != total {
		t.Errorf("rows = %d, want %d", len(store.rows), total)
	}
	if store.seq != total {
		t.Errorf("alert_seq = %d, want %d", store.seq, total)
	}
	if c := store.claims.Load(); c >= total/10 {
		t.Errorf("claim calls = %d, want far fewer than %d", c, total)
	} else {
		t.Logf("%d alerts claimed in %d calls to alert_seq", total, c)
	}
}

func TestClaimFailure_FailsWithoutWritingRows(t *testing.T) {
	store := newFakeStore()
	store.claimErr = errors.New("connection refused")
	a := newTestAllocator(t, store, nil, nil, testConfig())

	if _, err := a.Submit(context.Background(), "aws", "req", []model.Alert{alert("svc", "u")}); !errors.Is(err, ErrClaimFailed) {
		t.Fatalf("err = %v, want ErrClaimFailed", err)
	}
	if len(store.rows) != 0 {
		t.Errorf("rows = %d, want none: nothing was claimed", len(store.rows))
	}
}

func TestInsertFailure_WritesFillerAndFails(t *testing.T) {
	store := newFakeStore()
	store.failRealInsert = func(a string) bool { return strings.Contains(a, `"service":"bad"`) }
	notifier := &recordingNotifier{}
	a := newTestAllocator(t, store, notifier, nil, testConfig())

	ids, err := a.Submit(context.Background(), "prometheus", "req-9", []model.Alert{alert("good", "u1"), alert("bad", "u2")})
	if !errors.Is(err, ErrStoreFailed) {
		t.Fatalf("err = %v, want ErrStoreFailed (503)", err)
	}
	if len(ids) != 2 {
		t.Fatalf("ids = %v", ids)
	}
	good, bad := store.rows[ids[0]], store.rows[ids[1]]
	if !strings.Contains(good.alert, `"service":"good"`) {
		t.Errorf("good row = %q", good.alert)
	}
	if !strings.HasPrefix(bad.alert, "VOID: ") || bad.source != "prometheus" {
		t.Errorf("failed id should hold a filler row, got %+v", bad)
	}
	var probe model.Alert
	if json.Unmarshal([]byte(bad.alert), &probe) == nil {
		t.Error("filler row must not parse as an alert, or alerts-core would process it")
	}
	if n := store.inserts[ids[1]]; n != 4 {
		t.Errorf("insert calls on the failed id = %d, want 3 attempts + 1 filler", n)
	}
	if len(notifier.failures) != 1 {
		t.Fatalf("notifications = %d, want 1", len(notifier.failures))
	}
	f := notifier.failures[0]
	if f.AltID != ids[1] || f.Source != "prometheus" || f.RequestID != "req-9" || !f.FillerWritten || f.Alert.Service != "bad" {
		t.Errorf("failure = %+v", f)
	}
}

func TestInsertAndFillerBothFail_ReportsFillerMissing(t *testing.T) {
	store := newFakeStore()
	store.failAllInserts = true
	notifier := &recordingNotifier{}
	a := newTestAllocator(t, store, notifier, nil, testConfig())

	if _, err := a.Submit(context.Background(), "aws", "req", []model.Alert{alert("svc", "u")}); !errors.Is(err, ErrStoreFailed) {
		t.Fatalf("err = %v, want ErrStoreFailed", err)
	}
	if len(notifier.failures) != 1 || notifier.failures[0].FillerWritten {
		t.Errorf("failures = %+v, want one with FillerWritten=false", notifier.failures)
	}
	if n := store.inserts["ALT000000001"]; n != 6 {
		t.Errorf("insert calls = %d, want 3 attempts + 3 filler attempts", n)
	}
}

func TestFillerRetried_UntilWritten(t *testing.T) {
	store := newFakeStore()
	store.failRealInsert = func(string) bool { return true }
	store.failFillers = 2
	notifier := &recordingNotifier{}
	a := newTestAllocator(t, store, notifier, nil, testConfig())

	ids, err := a.Submit(context.Background(), "aws", "req", []model.Alert{alert("svc", "u")})
	if !errors.Is(err, ErrStoreFailed) {
		t.Fatalf("err = %v, want ErrStoreFailed", err)
	}
	if !strings.HasPrefix(store.rows[ids[0]].alert, fillerPrefix) {
		t.Errorf("row = %+v, want the filler after its third attempt", store.rows[ids[0]])
	}
	if len(notifier.failures) != 1 || !notifier.failures[0].FillerWritten {
		t.Errorf("failures = %+v, want FillerWritten=true", notifier.failures)
	}
}

func TestBatchSubmission_GetsConsecutiveIDsInOrder(t *testing.T) {
	store := newFakeStore()
	store.seq = 41
	a := newTestAllocator(t, store, nil, nil, testConfig())

	batch := []model.Alert{alert("a", "1"), alert("b", "2"), alert("c", "3")}
	ids, err := a.Submit(context.Background(), "prometheus", "req", batch)
	if err != nil {
		t.Fatal(err)
	}
	for i, id := range ids {
		if seqOf(t, id) != int64(42+i) {
			t.Fatalf("ids = %v, want ALT000000042..44", ids)
		}
		if !strings.Contains(store.rows[id].alert, `"service":"`+batch[i].Service+`"`) {
			t.Errorf("row %s holds the wrong alert: %s", id, store.rows[id].alert)
		}
	}
}

func TestLargeSubmissionIsNotSplit(t *testing.T) {
	store := newFakeStore()
	cfg := testConfig()
	cfg.MaxBatch = 2
	a := newTestAllocator(t, store, nil, nil, cfg)

	batch := make([]model.Alert, 5)
	for i := range batch {
		batch[i] = alert("svc", strconv.Itoa(i))
	}
	ids, err := a.Submit(context.Background(), "prometheus", "req", batch)
	if err != nil {
		t.Fatal(err)
	}
	if seqOf(t, ids[0]) != 1 || seqOf(t, ids[4]) != 5 || store.claims.Load() != 1 {
		t.Errorf("ids = %v, claims = %d; want one claim of 5 consecutive ids", ids, store.claims.Load())
	}
}

func TestMissingUniqueIdentifier_UsesOwnAltID(t *testing.T) {
	store := newFakeStore()
	a := newTestAllocator(t, store, nil, nil, testConfig())

	ids, err := a.Submit(context.Background(), "datadog", "req", []model.Alert{alert("svc", "")})
	if err != nil {
		t.Fatal(err)
	}
	var stored model.Alert
	if err := json.Unmarshal([]byte(store.rows[ids[0]].alert), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.UniqueIdentifier != ids[0] {
		t.Errorf("unique_identifier = %q, want %q", stored.UniqueIdentifier, ids[0])
	}
}

func TestWakesOncePerBatch(t *testing.T) {
	store := newFakeStore()
	waker := &countingWaker{}
	a := newTestAllocator(t, store, nil, waker, testConfig())

	if _, err := a.Submit(context.Background(), "aws", "req", []model.Alert{alert("a", "1"), alert("b", "2")}); err != nil {
		t.Fatal(err)
	}
	if n := waker.n.Load(); n != 1 {
		t.Errorf("wakes = %d, want 1 for one batch", n)
	}
}

func TestQueueFull_FailsImmediately(t *testing.T) {
	store := newFakeStore()
	store.claimGate = make(chan struct{})
	store.claimEntered = make(chan struct{}, 1)
	cfg := testConfig()
	cfg.QueueSize = 1
	a := newTestAllocator(t, store, nil, nil, cfg)

	results := make(chan error, 2)
	go func() {
		_, err := a.Submit(context.Background(), "aws", "r1", []model.Alert{alert("a", "1")})
		results <- err
	}()
	<-store.claimEntered // the claimer holds the first submission and is blocked on ClaimRange
	go func() {
		_, err := a.Submit(context.Background(), "aws", "r2", []model.Alert{alert("b", "2")})
		results <- err
	}()
	waitFor(t, func() bool { return len(a.queue) == 1 })

	start := time.Now()
	if _, err := a.Submit(context.Background(), "aws", "r3", []model.Alert{alert("c", "3")}); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("err = %v, want ErrQueueFull", err)
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Error("a full queue must fail immediately, not wait")
	}

	close(store.claimGate)
	for range 2 {
		if err := <-results; err != nil {
			t.Errorf("queued submissions should still succeed, got %v", err)
		}
	}
}

func TestShutdown_DrainsQueueThenRejects(t *testing.T) {
	store := newFakeStore()
	store.claimGate = make(chan struct{})
	store.claimEntered = make(chan struct{}, 1)
	cfg := testConfig()
	cfg.MaxBatch = 1 // the claimer holds exactly one, so the other four stay visibly queued
	a := New(slog.New(slog.NewTextHandler(io.Discard, nil)), store, nil, nil, cfg)

	results := make(chan error, 5)
	for i := range 5 {
		go func() {
			_, err := a.Submit(context.Background(), "aws", "r", []model.Alert{alert("s", strconv.Itoa(i))})
			results <- err
		}()
	}
	<-store.claimEntered
	waitFor(t, func() bool { return len(a.queue) == 4 })

	closed := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		closed <- a.Close(ctx)
	}()
	waitFor(t, func() bool { a.mu.RLock(); defer a.mu.RUnlock(); return a.closed })
	if _, err := a.Submit(context.Background(), "aws", "late", []model.Alert{alert("s", "x")}); !errors.Is(err, ErrShuttingDown) {
		t.Errorf("Submit after Close = %v, want ErrShuttingDown", err)
	}

	close(store.claimGate)
	for range 5 {
		if err := <-results; err != nil {
			t.Errorf("queued submission lost on shutdown: %v", err)
		}
	}
	if err := <-closed; err != nil {
		t.Errorf("Close = %v", err)
	}
	if len(store.rows) != 5 {
		t.Errorf("rows = %d, want 5", len(store.rows))
	}
}

func TestCancelledRequest_StillWritesItsRow(t *testing.T) {
	store := newFakeStore()
	store.claimGate = make(chan struct{})
	a := newTestAllocator(t, store, nil, nil, testConfig())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := a.Submit(ctx, "aws", "r", []model.Alert{alert("s", "u")}); done <- err }()
	waitFor(t, func() bool { return len(a.queue) == 0 })
	cancel()
	if err := <-done; !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}

	close(store.claimGate)
	closeCtx, cancelClose := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelClose()
	if err := a.Close(closeCtx); err != nil {
		t.Fatal(err)
	}
	if len(store.rows) != 1 {
		t.Errorf("rows = %d, want 1: a claimed id must get its row even after the client left", len(store.rows))
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestCloseTimeout_LogsUnwrittenIDs(t *testing.T) {
	store := newFakeStore()
	store.insertGate = make(chan struct{})
	var logs bytes.Buffer
	var logMu sync.Mutex
	logger := slog.New(slog.NewTextHandler(&lockedWriter{w: &logs, mu: &logMu}, nil))
	a := New(logger, store, nil, nil, testConfig())

	submitted := make(chan struct{})
	go func() {
		_, _ = a.Submit(context.Background(), "aws", "req", []model.Alert{alert("svc", "u1"), alert("svc", "u2")})
		close(submitted)
	}()
	for deadline := time.Now().Add(2 * time.Second); ; time.Sleep(time.Millisecond) {
		a.pendingMu.Lock()
		n := len(a.pending)
		a.pendingMu.Unlock()
		if n == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("ids were never claimed")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := a.Close(ctx); err == nil {
		t.Fatal("Close should time out while writes are blocked")
	}
	logMu.Lock()
	out := logs.String()
	logMu.Unlock()
	if !strings.Contains(out, "have no row") || !strings.Contains(out, "ALT000000001") || !strings.Contains(out, "ALT000000002") {
		t.Errorf("log should name the unwritten ids, got:\n%s", out)
	}
	close(store.insertGate)
	<-submitted
}

type lockedWriter struct {
	w  io.Writer
	mu *sync.Mutex
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// TestClaimer_ClaimsOnlyForFreeWriterSlots confirms write_concurrency gates concurrent claimed groups; MaxBatch=1 forces one alert per group so the assertion below is deterministic.
func TestClaimer_ClaimsOnlyForFreeWriterSlots(t *testing.T) {
	store := newFakeStore()
	store.insertGate = make(chan struct{})
	cfg := testConfig()
	cfg.WriteConcurrency = 4
	cfg.MaxBatch = 1
	a := newTestAllocator(t, store, nil, nil, cfg)

	const burst = 10
	var wg sync.WaitGroup
	errs := make(chan error, burst)
	for i := range burst {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := a.Submit(context.Background(), "aws", "req", []model.Alert{alert("svc", string(rune('a'+i)))})
			errs <- err
		}()
	}
	waitFor(t, func() bool { store.mu.Lock(); defer store.mu.Unlock(); return store.seq == 4 })
	time.Sleep(50 * time.Millisecond)
	store.mu.Lock()
	claimed := store.seq
	store.mu.Unlock()
	if claimed != int64(cfg.WriteConcurrency) {
		t.Fatalf("claimed %d ids with %d writer slots busy, want at most %d", claimed, cfg.WriteConcurrency, cfg.WriteConcurrency)
	}

	close(store.insertGate)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("submit: %v", err)
		}
	}
	for s := int64(1); s <= burst; s++ {
		if row, ok := store.rows[postgres.FormatID(s)]; !ok || strings.HasPrefix(row.alert, fillerPrefix) {
			t.Errorf("%s has no real row", postgres.FormatID(s))
		}
	}
	if b := a.QueueBytes(); b != 0 {
		t.Errorf("queue bytes = %d after the burst, want 0", b)
	}
}

func TestWriteBatch_OneInsertBatchCallPerClaimedGroup(t *testing.T) {
	store := newFakeStore()
	store.claimDelay = 2 * time.Millisecond // lets all submissions queue up before one claim
	a := newTestAllocator(t, store, nil, nil, testConfig())

	const total = 50
	var wg sync.WaitGroup
	for i := range total {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := a.Submit(context.Background(), "aws", "req", []model.Alert{alert("svc", strconv.Itoa(i))})
			if err != nil {
				t.Errorf("Submit: %v", err)
			}
		}()
	}
	wg.Wait()

	if len(store.rows) != total {
		t.Errorf("rows = %d, want %d", len(store.rows), total)
	}
	if calls := store.insertBatch.Load(); calls >= total/2 {
		t.Errorf("InsertBatch calls = %d, want far fewer than %d (one per claimed group, not one per alert)", calls, total)
	} else {
		t.Logf("%d alerts written in %d InsertBatch calls", total, calls)
	}
}

func TestOversizeSubmission_ClaimedAloneAndStored(t *testing.T) {
	store := newFakeStore()
	cfg := testConfig()
	cfg.WriteConcurrency = 2
	a := newTestAllocator(t, store, nil, nil, cfg)

	batch := make([]model.Alert, 5)
	for i := range batch {
		batch[i] = alert("svc", string(rune('a'+i)))
	}
	ids, err := a.Submit(context.Background(), "prometheus", "req", batch)
	if err != nil || len(ids) != 5 {
		t.Fatalf("ids = %v, err = %v", ids, err)
	}
	for i, id := range ids {
		if seqOf(t, id) != int64(i+1) {
			t.Errorf("ids = %v, want consecutive from 1", ids)
		}
	}
}

func TestQueueBytes_LimitAndRelease(t *testing.T) {
	big := alert("svc", "u")
	big.Description = strings.Repeat("x", 200)

	t.Run("over the limit", func(t *testing.T) {
		store := newFakeStore()
		cfg := testConfig()
		cfg.QueueMaxBytes = 100
		a := newTestAllocator(t, store, nil, nil, cfg)
		if _, err := a.Submit(context.Background(), "aws", "req", []model.Alert{big}); !errors.Is(err, ErrQueueBytesFull) {
			t.Fatalf("err = %v, want ErrQueueBytesFull", err)
		}
		if store.claims.Load() != 0 || a.QueueBytes() != 0 {
			t.Errorf("claimed %d times, %d bytes held; want nothing", store.claims.Load(), a.QueueBytes())
		}
	})

	cases := map[string]func(*fakeStore){
		"stored":        func(*fakeStore) {},
		"filler path":   func(s *fakeStore) { s.failRealInsert = func(string) bool { return true } },
		"claim failure": func(s *fakeStore) { s.claimErr = errors.New("connection refused") },
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			store := newFakeStore()
			setup(store)
			a := newTestAllocator(t, store, &recordingNotifier{}, nil, testConfig())
			_, _ = a.Submit(context.Background(), "aws", "req", []model.Alert{big})
			if b := a.QueueBytes(); b != 0 {
				t.Errorf("queue bytes = %d, want 0", b)
			}
		})
	}

	t.Run("shutdown", func(t *testing.T) {
		store := newFakeStore()
		store.insertGate = make(chan struct{})
		a := New(slog.New(slog.NewTextHandler(io.Discard, nil)), store, nil, nil, testConfig())
		done := make(chan struct{})
		go func() {
			_, _ = a.Submit(context.Background(), "aws", "req", []model.Alert{big})
			close(done)
		}()
		waitFor(t, func() bool { return a.QueueBytes() > 0 })
		closed := make(chan error, 1)
		go func() { closed <- a.Close(context.Background()) }()
		close(store.insertGate)
		<-done
		if err := <-closed; err != nil {
			t.Fatal(err)
		}
		if b := a.QueueBytes(); b != 0 {
			t.Errorf("queue bytes = %d after shutdown, want 0", b)
		}
		if _, err := a.Submit(context.Background(), "aws", "req", []model.Alert{big}); !errors.Is(err, ErrShuttingDown) || a.QueueBytes() != 0 {
			t.Errorf("submit after close: err = %v, bytes = %d", err, a.QueueBytes())
		}
	})
}

func TestSizeOf_SharedDescriptionCountedOnce(t *testing.T) {
	desc := strings.Repeat("d", 1000)
	one := alert("svc", "u")
	one.Description = desc
	batch := []model.Alert{one, one, one}
	fields := sizeOf([]model.Alert{one}) - int64(len(desc))
	if got, want := sizeOf(batch), 3*fields+int64(len(desc)); got != want {
		t.Errorf("sizeOf = %d, want %d (description counted once)", got, want)
	}
}
