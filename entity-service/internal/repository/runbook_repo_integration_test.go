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

// This is an integration test: it applies migrations 0241 and 0242 in a private schema and runs
// RunbookRepository against it. Skipped without a DSN:
//
//	RUNBOOK_TEST_DSN=postgres://... go test ./internal/repository/ -run Runbook

package repository_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

// runbookTestPool creates a private schema with migration 0241 applied, runs seed against it, applies 0242 twice and returns a pool on that schema.
func runbookTestPool(t *testing.T, seed string) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("RUNBOOK_TEST_DSN")
	if dsn == "" {
		t.Skip("RUNBOOK_TEST_DSN not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(admin.Close)
	schema := fmt.Sprintf("runbook_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE") })

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect to schema: %v", err)
	}
	t.Cleanup(pool.Close)

	apply := func(file string) {
		sql, err := os.ReadFile("../../migrations/" + file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", file, err)
		}
	}
	apply("0241_runbook.sql")
	if seed != "" {
		if _, err := pool.Exec(ctx, seed); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	apply("0242_runbook_alerts.sql")
	apply("0242_runbook_alerts.sql")
	return pool
}

func isConflict(err error) bool {
	var c *apierror.ConflictError
	return errors.As(err, &c)
}

func TestRunbookIntegration_MigrationMovesEachEntryIntoItsOwnRunbook(t *testing.T) {
	pool := runbookTestPool(t, `INSERT INTO runbook (alert_name, runbook_url, created_by, updated_by)
		VALUES ('gw-5xx', 'https://docs.example.com/gw', 's', 's'), (' JVM Heap ', NULL, 's', 's')`)
	got, err := repository.NewRunbookRepository(pool).List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d runbooks, want 2", len(got))
	}
	if got[0].Name != "gw-5xx" || got[0].RunbookURL == nil || !reflect.DeepEqual(got[0].Alerts, []string{"gw-5xx"}) {
		t.Fatalf("first = %+v", got[0])
	}
	if got[1].Name != "JVM Heap" || got[1].RunbookURL != nil || !reflect.DeepEqual(got[1].Alerts, []string{"JVM Heap"}) {
		t.Fatalf("second = %+v", got[1])
	}
}

func TestRunbookIntegration_CreateUpdateDelete(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewRunbookRepository(runbookTestPool(t, ""))

	gw, err := repo.Create(ctx, "a@x", domain.SaveRunbookRequest{Name: "Gateway", RunbookURL: strp("https://d/gw"), Alerts: []string{"gw-latency", "Gateway 5xx Spike"}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !reflect.DeepEqual(gw.Alerts, []string{"Gateway 5xx Spike", "gw-latency"}) || gw.CreatedBy != "a@x" {
		t.Fatalf("created = %+v", gw)
	}

	if _, err := repo.Create(ctx, "a@x", domain.SaveRunbookRequest{Name: "gateway", Alerts: []string{"other"}}); !isConflict(err) {
		t.Fatalf("same name, other case: err = %v, want a conflict", err)
	}
	_, err = repo.Create(ctx, "a@x", domain.SaveRunbookRequest{Name: "JVM", Alerts: []string{"GW-LATENCY"}})
	if !isConflict(err) || err.Error() != "alert gw-latency already belongs to runbook Gateway" {
		t.Fatalf("taken alert: err = %v", err)
	}

	updated, err := repo.Update(ctx, "b@x", gw.ID, domain.SaveRunbookRequest{Name: "Gateway", Alerts: []string{"gw-latency", "gw-timeout"}})
	if err != nil {
		t.Fatalf("Update keeping its own alert: %v", err)
	}
	if !reflect.DeepEqual(updated.Alerts, []string{"gw-latency", "gw-timeout"}) || updated.RunbookURL != nil || updated.UpdatedBy != "b@x" {
		t.Fatalf("updated = %+v", updated)
	}
	if missing, err := repo.Update(ctx, "b@x", "00000000-0000-4000-8000-000000000000", domain.SaveRunbookRequest{Name: "N", Alerts: []string{"n"}}); err != nil || missing != nil {
		t.Fatalf("Update unknown id = %v, %v; want nil, nil", missing, err)
	}

	if ok, err := repo.Delete(ctx, gw.ID); err != nil || !ok {
		t.Fatalf("Delete = %v, %v", ok, err)
	}
	if _, err := repo.Create(ctx, "a@x", domain.SaveRunbookRequest{Name: "JVM", Alerts: []string{"gw-latency"}}); err != nil {
		t.Fatalf("deleting a runbook must free its alerts: %v", err)
	}
}

func TestRunbookIntegration_ImportUpsertsByNameAndMovesAlerts(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewRunbookRepository(runbookTestPool(t, ""))
	if _, err := repo.Create(ctx, "a@x", domain.SaveRunbookRequest{Name: "Gateway", Alerts: []string{"gw-5xx", "gw-latency"}}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	res, err := repo.Import(ctx, "i@x", []domain.SaveRunbookRequest{
		{Name: "GATEWAY", RunbookURL: strp("https://d/gw"), Alerts: []string{"gw-5xx"}},
		{Name: "Latency", Alerts: []string{"gw-latency", "db-latency"}},
	})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if res.Created != 1 || res.Updated != 1 {
		t.Fatalf("import = %+v, want 1 created and 1 updated", res)
	}
	got, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := map[string][]string{"Gateway": {"gw-5xx"}, "Latency": {"db-latency", "gw-latency"}}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for _, rb := range got {
		if !reflect.DeepEqual(rb.Alerts, want[rb.Name]) {
			t.Fatalf("%s alerts = %v, want %v", rb.Name, rb.Alerts, want[rb.Name])
		}
	}
}

func TestRunbookIntegration_ImportNeverEmptiesARunbookItDoesNotName(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewRunbookRepository(runbookTestPool(t, ""))
	if _, err := repo.Create(ctx, "a@x", domain.SaveRunbookRequest{Name: "Disk", Alerts: []string{"disk-low", "disk-full"}}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err := repo.Import(ctx, "i@x", []domain.SaveRunbookRequest{{Name: "Storage", Alerts: []string{"DISK-LOW", "disk-full"}}})
	if !isConflict(err) || err.Error() != "importing would leave runbook Disk without alerts" {
		t.Fatalf("err = %v, want the conflict naming Disk", err)
	}
	got, _ := repo.List(ctx)
	if len(got) != 1 || got[0].Name != "Disk" || len(got[0].Alerts) != 2 {
		t.Fatalf("a refused import must change nothing, got %+v", got)
	}

	if _, err := repo.Import(ctx, "i@x", []domain.SaveRunbookRequest{
		{Name: "Storage", Alerts: []string{"disk-low", "disk-full"}},
		{Name: "Disk", Alerts: []string{"disk-io"}},
	}); err != nil {
		t.Fatalf("a runbook the import names may lose its alerts to another entry: %v", err)
	}
	got, _ = repo.List(ctx)
	want := map[string][]string{"Disk": {"disk-io"}, "Storage": {"disk-full", "disk-low"}}
	for _, rb := range got {
		if !reflect.DeepEqual(rb.Alerts, want[rb.Name]) {
			t.Fatalf("%s alerts = %v, want %v", rb.Name, rb.Alerts, want[rb.Name])
		}
	}
}
