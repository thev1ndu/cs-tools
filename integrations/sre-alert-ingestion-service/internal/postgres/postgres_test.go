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

//go:build integration

package postgres

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"
)

// TestInsertBatch_IsolatesFailuresPerRow runs InsertBatch against a real local Postgres (see README.md's "Postgres integration tests" section) to prove one row's failure never aborts the rest.
func TestInsertBatch_IsolatesFailuresPerRow(t *testing.T) {
	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv: %v", err)
	}
	admin, err := Connect(cfg, 10*time.Second, 5*time.Second)
	if err != nil {
		t.Fatalf("connect to %s: %v", cfg.Database, err)
	}
	defer admin.Close()

	dbName := fmt.Sprintf("ingestion_test_%d", rand.Int63())
	if _, err := admin.Exec(context.Background(), "CREATE DATABASE "+dbName); err != nil {
		t.Fatalf("create database %s: %v", dbName, err)
	}
	defer func() {
		_, _ = admin.Exec(context.Background(), "DROP DATABASE "+dbName)
	}()

	testCfg := cfg
	testCfg.Database = dbName
	pool, err := Connect(testCfg, 10*time.Second, 5*time.Second)
	if err != nil {
		t.Fatalf("connect to %s: %v", dbName, err)
	}
	defer pool.Close()
	if _, err := pool.Exec(context.Background(), `CREATE TABLE alerts (
		id text PRIMARY KEY,
		source text NOT NULL,
		alert jsonb NOT NULL,
		created_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	store := NewStore(pool, 5*time.Second, 5*time.Second)

	// Seed one row so a later insert under the same id collides and fails, while the rest still land.
	if err := store.Insert(context.Background(), "ALT000000002", "existing-source", []byte(`{"service":"pre-existing"}`)); err != nil {
		t.Fatalf("seed existing row: %v", err)
	}

	rows := []InsertRow{
		{ID: "ALT000000001", Source: "aws", Alert: []byte(`{"service":"one"}`)},
		{ID: "ALT000000002", Source: "aws", Alert: []byte(`{"service":"two"}`)}, // collides with the seeded row
		{ID: "ALT000000003", Source: "aws", Alert: []byte(`{"service":"three"}`)},
	}
	errs := store.InsertBatch(context.Background(), rows)
	if len(errs) != 3 {
		t.Fatalf("errs = %v, want 3 entries", errs)
	}
	if errs[0] != nil || errs[2] != nil {
		t.Errorf("rows 1 and 3 should have been stored: errs = %v", errs)
	}
	if errs[1] == nil {
		t.Error("row 2 should have failed on its primary key collision")
	}

	for _, id := range []string{"ALT000000001", "ALT000000003"} {
		var got []byte
		if err := pool.QueryRow(context.Background(), "SELECT alert FROM alerts WHERE id = $1", id).Scan(&got); err != nil {
			t.Errorf("row %s was not stored despite a nil error: %v", id, err)
		}
	}
}
