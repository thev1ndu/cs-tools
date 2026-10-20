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
	"reflect"
	"testing"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

type recordingRunbookRepo struct {
	repository.RunbookRepository
	created  domain.SaveRunbookRequest
	imported []domain.SaveRunbookRequest
}

func (r *recordingRunbookRepo) Create(_ context.Context, _ string, req domain.SaveRunbookRequest) (domain.Runbook, error) {
	r.created = req
	return domain.Runbook{}, nil
}

func (r *recordingRunbookRepo) Import(_ context.Context, _ string, reqs []domain.SaveRunbookRequest) (domain.ImportRunbooksResponse, error) {
	r.imported = reqs
	return domain.ImportRunbooksResponse{}, nil
}

func TestRunbookCreate_TrimsAndDropsBlankAndRepeatedAlerts(t *testing.T) {
	repo := &recordingRunbookRepo{}
	_, err := NewRunbookService(repo).Create(context.Background(), domain.SaveRunbookRequest{
		Name: "  Gateway ", RunbookURL: strp("  "), Alerts: []string{" Alert 1 ", "", "alert 1", "Alert 2"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	want := domain.SaveRunbookRequest{Name: "Gateway", Alerts: []string{"Alert 1", "Alert 2"}}
	if !reflect.DeepEqual(repo.created, want) {
		t.Fatalf("saved %+v, want %+v", repo.created, want)
	}
}

func TestRunbookCreate_Refusals(t *testing.T) {
	long := make([]byte, maxRunbookAlertNameLength+1)
	for i := range long {
		long[i] = 'a'
	}
	for name, req := range map[string]domain.SaveRunbookRequest{
		"no name":       {Alerts: []string{"a"}},
		"no alert":      {Name: "N", Alerts: []string{" "}},
		"long alert":    {Name: "N", Alerts: []string{string(long)}},
		"relative link": {Name: "N", Alerts: []string{"a"}, RunbookURL: strp("docs/a")},
	} {
		var ve *apierror.ValidationError
		if _, err := NewRunbookService(&recordingRunbookRepo{}).Create(context.Background(), req); !errors.As(err, &ve) {
			t.Errorf("%s: err = %v, want a ValidationError", name, err)
		}
	}
}

func TestRunbookImport_LaterEntriesWin(t *testing.T) {
	repo := &recordingRunbookRepo{}
	_, err := NewRunbookService(repo).Import(context.Background(), domain.ImportRunbooksRequest{Runbooks: []domain.SaveRunbookRequest{
		{Name: "Gateway", Alerts: []string{"gw-5xx", "shared"}},
		{Name: "Latency", Alerts: []string{"SHARED", "db"}},
		{Name: "gateway", Alerts: []string{"gw-5xx", "gw-timeout"}},
	}})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	want := []domain.SaveRunbookRequest{
		{Name: "gateway", Alerts: []string{"gw-5xx", "gw-timeout"}},
		{Name: "Latency", Alerts: []string{"SHARED", "db"}},
	}
	if !reflect.DeepEqual(repo.imported, want) {
		t.Fatalf("imported %+v, want %+v", repo.imported, want)
	}
}

func TestRunbookImport_RefusesAnEntryLeftWithoutAlerts(t *testing.T) {
	repo := &recordingRunbookRepo{}
	_, err := NewRunbookService(repo).Import(context.Background(), domain.ImportRunbooksRequest{Runbooks: []domain.SaveRunbookRequest{
		{Name: "A", Alerts: []string{"x"}},
		{Name: "B", Alerts: []string{"X"}},
	}})
	var ve *apierror.ValidationError
	if !errors.As(err, &ve) || repo.imported != nil {
		t.Fatalf("err = %v, imported = %v; want a ValidationError and nothing imported", err, repo.imported)
	}
}

func TestRunbookMergeAndUnmerge_Refusals(t *testing.T) {
	svc := NewRunbookService(&recordingRunbookRepo{})
	id := "11111111-1111-4111-8111-111111111111"
	for name, req := range map[string]domain.MergeRunbooksRequest{
		"one runbook":   {RunbookIDs: []string{id, " " + id + " "}, Name: "N"},
		"not an id":     {RunbookIDs: []string{id, "nope"}, Name: "N"},
		"no name":       {RunbookIDs: []string{id, "22222222-2222-4222-8222-222222222222"}},
		"relative link": {RunbookIDs: []string{id, "22222222-2222-4222-8222-222222222222"}, Name: "N", RunbookURL: strp("docs")},
	} {
		var ve *apierror.ValidationError
		if _, err := svc.Merge(context.Background(), req); !errors.As(err, &ve) {
			t.Errorf("merge, %s: err = %v, want a ValidationError", name, err)
		}
	}
	var ve *apierror.ValidationError
	if _, err := svc.Unmerge(context.Background(), id, domain.UnmergeRunbookAlertsRequest{Alerts: []string{" "}}); !errors.As(err, &ve) {
		t.Errorf("unmerge with no alert: err = %v, want a ValidationError", err)
	}
	if _, err := svc.Unmerge(context.Background(), "nope", domain.UnmergeRunbookAlertsRequest{Alerts: []string{"a"}}); !errors.As(err, &ve) {
		t.Errorf("unmerge with a bad id: err = %v, want a ValidationError", err)
	}
}
