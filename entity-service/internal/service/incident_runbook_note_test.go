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

package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
)

type fakeRunbookLister struct {
	runbooks []domain.Runbook
	err      error
}

func (f fakeRunbookLister) List(context.Context) ([]domain.Runbook, error) {
	return f.runbooks, f.err
}

var testRunbooks = []domain.Runbook{
	{AlertName: "example-gw-CriticalHTTP5xxCountLimitExceeded-prod-001", RunbookURL: strp("https://docs.example.com/gw-5xx")},
	{AlertName: "example-gw-CriticalHTTP5xx", RunbookURL: strp("https://docs.example.com/short")},
	{AlertName: "Icinga Alert: demo-node-01 / JVM Heap"},
}

func TestRunbookNote(t *testing.T) {
	linked := runbookNote(testRunbooks, domain.CreateIncidentRequest{
		Subject: "[Alert]Fired:Sev0 Azure Monitor Alert EXAMPLE-gw-CriticalHTTP5xxCountLimitExceeded-prod-001 on log-002"})
	for _, want := range []string{
		"<table style='border:1px solid #dcdcdc; border-collapse:collapse;",
		">Alert</th><th style=",
		">Runbook</th></tr>",
		">example-gw-CriticalHTTP5xxCountLimitExceeded-prod-001</td>",
		`<a href="https://docs.example.com/gw-5xx" target="_blank" rel="noopener noreferrer">https://docs.example.com/gw-5xx</a></td></tr></table>`,
	} {
		if !strings.Contains(linked, want) {
			t.Fatalf("linked note %q does not contain %q", linked, want)
		}
	}
	if !strings.HasPrefix(linked, "<table") || strings.Contains(linked, "[code]") || strings.Contains(linked, "alert_name") {
		t.Fatalf("note must be the Alert | Runbook table alone: %q", linked)
	}

	short := runbookNote(testRunbooks, domain.CreateIncidentRequest{Subject: "example-gw-CriticalHTTP5xx spike"})
	if !strings.Contains(short, "https://docs.example.com/short") {
		t.Fatalf("shorter entry did not match on its own: %q", short)
	}

	unlinked := runbookNote(testRunbooks, domain.CreateIncidentRequest{Subject: "Icinga Alert: demo-node-01 / JVM Heap"})
	if !strings.Contains(unlinked, ">Icinga Alert: demo-node-01 / JVM Heap</td>") || !strings.Contains(unlinked, ">"+noRunbookNoteText+"</td>") {
		t.Fatalf("unlinked note = %q", unlinked)
	}

	if got := runbookNote(testRunbooks, domain.CreateIncidentRequest{Subject: "unknown alert", CorrelationID: strp("fp-1")}); got != "<p>"+noRunbookNoteText+"</p>" {
		t.Fatalf("alert-born no-match note = %q", got)
	}
	for _, corr := range []*string{nil, strp("  ")} {
		if got := runbookNote(testRunbooks, domain.CreateIncidentRequest{Subject: "Login page is slow", CorrelationID: corr}); got != "" {
			t.Fatalf("customer incident got a note: %q", got)
		}
	}
}

func TestRunbookNote_EscapesCatalogValues(t *testing.T) {
	rbs := []domain.Runbook{{AlertName: "<script>x</script>", RunbookURL: strp(`https://docs.example.com/a?x="><img src=y>`)}}
	got := runbookNote(rbs, domain.CreateIncidentRequest{Subject: "fired <script>x</script> now"})
	if strings.Contains(got, "<script>") || strings.Contains(got, "<img") || strings.Contains(got, `"><`) {
		t.Fatalf("catalog values reached the note unescaped: %q", got)
	}
}

func TestWithRunbookNote_AppendsAfterCallerNotes(t *testing.T) {
	req := domain.CreateIncidentRequest{Subject: "Icinga Alert: demo-node-01 / JVM Heap", WorkNotes: strp("<p>Created by alert-core.</p>")}
	got := withRunbookNote(context.Background(), req, fakeRunbookLister{runbooks: testRunbooks})
	want := "<p>Created by alert-core.</p><table"
	if got.WorkNotes == nil || !strings.HasPrefix(*got.WorkNotes, want) {
		t.Fatalf("WorkNotes = %v, want %q", got.WorkNotes, want)
	}
}

func TestWithRunbookNote_CatalogFailureLeavesRequestAlone(t *testing.T) {
	req := domain.CreateIncidentRequest{Subject: "Icinga Alert: demo-node-01 / JVM Heap", WorkNotes: strp("keep")}
	got := withRunbookNote(context.Background(), req, fakeRunbookLister{err: errors.New("db down")})
	if got.WorkNotes == nil || *got.WorkNotes != "keep" {
		t.Fatalf("WorkNotes = %v, want unchanged", got.WorkNotes)
	}
}

func TestWithRunbookNote_NoCatalogIsNoOp(t *testing.T) {
	req := domain.CreateIncidentRequest{Subject: "x", CorrelationID: strp("fp")}
	if got := withRunbookNote(context.Background(), req, nil); got.WorkNotes != nil {
		t.Fatalf("WorkNotes = %v, want nil", *got.WorkNotes)
	}
}

func TestNormalizeRunbook(t *testing.T) {
	got, err := normalizeRunbook(domain.SaveRunbookRequest{AlertName: "  a  ", RunbookURL: strp("  ")})
	if err != nil || got.AlertName != "a" || got.RunbookURL != nil {
		t.Fatalf("normalizeRunbook = %+v, %v; want trimmed name and nil link", got, err)
	}
	for _, bad := range []domain.SaveRunbookRequest{
		{AlertName: " "},
		{AlertName: "a", RunbookURL: strp("javascript:alert(1)")},
		{AlertName: "a", RunbookURL: strp("docs.example.com/x")},
	} {
		if _, err := normalizeRunbook(bad); err == nil {
			t.Fatalf("normalizeRunbook(%+v) accepted, want a validation error", bad)
		}
	}
}
