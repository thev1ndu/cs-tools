// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
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

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/apierror"
)

type fakeRunbookClient struct {
	body []byte
	id   string
	err  error
}

func (f *fakeRunbookClient) ListRunbooks(context.Context) ([]byte, error) {
	return []byte(`{"runbooks":[]}`), f.err
}

func (f *fakeRunbookClient) CreateRunbook(_ context.Context, body []byte) ([]byte, error) {
	f.body = body
	return []byte(`{"id":"x"}`), f.err
}

func (f *fakeRunbookClient) UpdateRunbook(_ context.Context, id string, body []byte) ([]byte, error) {
	f.id, f.body = id, body
	return []byte(`{"id":"x"}`), f.err
}

func (f *fakeRunbookClient) DeleteRunbook(_ context.Context, id string) ([]byte, error) {
	f.id = id
	return nil, f.err
}

func (f *fakeRunbookClient) ImportRunbooks(_ context.Context, body []byte) ([]byte, error) {
	f.body = body
	return []byte(`{"created":1,"updated":0}`), f.err
}

func TestRunbookHandler_CreateRebuildsTheBody(t *testing.T) {
	fake := &fakeRunbookClient{}
	h := NewRunbookHandler(fake)
	r := withUser(httptest.NewRequest(http.MethodPost, "/runbooks",
		strings.NewReader(`{"name":"A","runbookUrl":"https://x.example","alerts":["a"],"extra":"dropped"}`)))
	w := httptest.NewRecorder()
	h.CreateRunbook(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body %s", w.Code, w.Body.String())
	}
	if got := string(fake.body); got != `{"name":"A","runbookUrl":"https://x.example","alerts":["a"]}` {
		t.Fatalf("forwarded body = %s", got)
	}
}

func TestRunbookHandler_RefusesAMissingNameOrAlert(t *testing.T) {
	for _, body := range []string{`{"name":"  ","alerts":["a"]}`, `{"name":"A","alerts":["  "]}`, `{"name":"A"}`} {
		fake := &fakeRunbookClient{}
		w := httptest.NewRecorder()
		NewRunbookHandler(fake).CreateRunbook(w, withUser(httptest.NewRequest(http.MethodPost, "/runbooks", strings.NewReader(body))))
		if w.Code != http.StatusBadRequest || fake.body != nil {
			t.Fatalf("%s: status = %d, forwarded = %s; want 400 and nothing forwarded", body, w.Code, fake.body)
		}
	}
}

func TestRunbookHandler_UpdateRefusesBadID(t *testing.T) {
	fake := &fakeRunbookClient{}
	r := withUser(httptest.NewRequest(http.MethodPut, "/runbooks/nope", strings.NewReader(`{"name":"A","alerts":["a"]}`)))
	r.SetPathValue("id", "nope")
	w := httptest.NewRecorder()
	NewRunbookHandler(fake).UpdateRunbook(w, r)
	if w.Code != http.StatusBadRequest || fake.id != "" {
		t.Fatalf("status = %d, id = %q; want 400 and no call", w.Code, fake.id)
	}
}

func TestRunbookHandler_ConflictKeepsEntityMessage(t *testing.T) {
	fake := &fakeRunbookClient{err: &apierror.Error{StatusCode: http.StatusConflict,
		Body: `{"code":409,"message":"alert a already belongs to runbook A"}`}}
	w := httptest.NewRecorder()
	NewRunbookHandler(fake).CreateRunbook(w, withUser(httptest.NewRequest(http.MethodPost, "/runbooks", strings.NewReader(`{"name":"A","alerts":["a"]}`))))
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "already belongs") {
		t.Fatalf("status = %d, body %s; want 409 with the entity message", w.Code, w.Body.String())
	}
}

func TestRunbookHandler_UnauthenticatedIs401(t *testing.T) {
	w := httptest.NewRecorder()
	NewRunbookHandler(&fakeRunbookClient{}).ListRunbooks(w, httptest.NewRequest(http.MethodGet, "/runbooks", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestRunbookHandler_ImportAcceptsBodiesOverTheGeneralLimit(t *testing.T) {
	fake := &fakeRunbookClient{}
	var b strings.Builder
	b.WriteString(`{"runbooks":[`)
	for i := 0; i < 1000; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"name":"` + strings.Repeat("n", 1000) + `","alerts":["` + strings.Repeat("a", 1000) + `"],"runbookUrl":"https://docs.example.com/` + strings.Repeat("x", 900) + `"}`)
	}
	b.WriteString(`]}`)
	if b.Len() <= maxRequestBodyBytes {
		t.Fatalf("test body is %d bytes; it must exceed the general %d byte limit", b.Len(), maxRequestBodyBytes)
	}
	w := httptest.NewRecorder()
	NewRunbookHandler(fake).ImportRunbooks(w, withUser(httptest.NewRequest(http.MethodPost, "/runbooks/import", strings.NewReader(b.String()))))
	if w.Code != http.StatusOK || fake.body == nil {
		t.Fatalf("status = %d, forwarded = %v; want 200 and the import forwarded", w.Code, fake.body != nil)
	}
}
