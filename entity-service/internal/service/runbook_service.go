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
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

const (
	maxRunbookNameLength      = 1000
	maxRunbookAlertNameLength = 1000
	maxRunbookAlerts          = 500
	maxRunbookURLLength       = 2000
	maxRunbookImportEntries   = 5000
)

type runbookService struct {
	repo repository.RunbookRepository
}

// NewRunbookService constructs a RunbookService.
func NewRunbookService(repo repository.RunbookRepository) RunbookService {
	return &runbookService{repo: repo}
}

func (s *runbookService) List(ctx context.Context) (domain.RunbookList, error) {
	rbs, err := s.repo.List(ctx)
	if err != nil {
		return domain.RunbookList{}, err
	}
	return domain.RunbookList{Runbooks: rbs}, nil
}

func (s *runbookService) Create(ctx context.Context, req domain.SaveRunbookRequest) (domain.Runbook, error) {
	clean, err := normalizeRunbook(req)
	if err != nil {
		return domain.Runbook{}, err
	}
	return s.repo.Create(ctx, actorOf(ctx), clean)
}

func (s *runbookService) Update(ctx context.Context, id string, req domain.SaveRunbookRequest) (domain.Runbook, error) {
	if err := validateRunbookID(id); err != nil {
		return domain.Runbook{}, err
	}
	clean, err := normalizeRunbook(req)
	if err != nil {
		return domain.Runbook{}, err
	}
	rb, err := s.repo.Update(ctx, actorOf(ctx), id, clean)
	if err != nil {
		return domain.Runbook{}, err
	}
	if rb == nil {
		return domain.Runbook{}, &apierror.NotFoundError{Msg: "runbook not found"}
	}
	return *rb, nil
}

func (s *runbookService) Delete(ctx context.Context, id string) error {
	if err := validateRunbookID(id); err != nil {
		return err
	}
	ok, err := s.repo.Delete(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return &apierror.NotFoundError{Msg: "runbook not found"}
	}
	return nil
}

func (s *runbookService) Import(ctx context.Context, req domain.ImportRunbooksRequest) (domain.ImportRunbooksResponse, error) {
	if len(req.Runbooks) == 0 {
		return domain.ImportRunbooksResponse{}, &apierror.ValidationError{Msg: "runbooks must have at least one entry"}
	}
	if len(req.Runbooks) > maxRunbookImportEntries {
		return domain.ImportRunbooksResponse{}, &apierror.ValidationError{Msg: fmt.Sprintf("runbooks must have at most %d entries", maxRunbookImportEntries)}
	}
	// Later entries win: a repeated name replaces the earlier entry, and a repeated alert leaves the earlier runbook.
	order := []string{}
	byName := map[string]domain.SaveRunbookRequest{}
	for i, r := range req.Runbooks {
		clean, err := normalizeRunbook(r)
		if err != nil {
			return domain.ImportRunbooksResponse{}, &apierror.ValidationError{Msg: fmt.Sprintf("runbooks[%d]: %s", i, err.Error())}
		}
		key := strings.ToLower(clean.Name)
		if _, seen := byName[key]; !seen {
			order = append(order, key)
		}
		byName[key] = clean
	}
	alertOwner := map[string]string{}
	for _, key := range order {
		for _, a := range byName[key].Alerts {
			alertOwner[strings.ToLower(a)] = key
		}
	}
	entries := make([]domain.SaveRunbookRequest, 0, len(order))
	for _, key := range order {
		e := byName[key]
		kept := make([]string, 0, len(e.Alerts))
		for _, a := range e.Alerts {
			if alertOwner[strings.ToLower(a)] == key {
				kept = append(kept, a)
			}
		}
		e.Alerts = kept
		entries = append(entries, e)
	}
	return s.repo.Import(ctx, actorOf(ctx), entries)
}

func validateRunbookID(id string) error {
	return validateUUIDs("id", []string{id})
}

// normalizeRunbook trims every field, turns a blank link into nil, drops blank and repeated alerts and checks what is left.
func normalizeRunbook(req domain.SaveRunbookRequest) (domain.SaveRunbookRequest, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return req, &apierror.ValidationError{Msg: "name is required"}
	}
	if utf8.RuneCountInString(name) > maxRunbookNameLength {
		return req, &apierror.ValidationError{Msg: fmt.Sprintf("name must be at most %d characters", maxRunbookNameLength)}
	}
	out := domain.SaveRunbookRequest{Name: name, Alerts: []string{}}
	seen := map[string]bool{}
	for _, a := range req.Alerts {
		alert := strings.TrimSpace(a)
		if alert == "" || seen[strings.ToLower(alert)] {
			continue
		}
		if utf8.RuneCountInString(alert) > maxRunbookAlertNameLength {
			return req, &apierror.ValidationError{Msg: fmt.Sprintf("each alert must be at most %d characters", maxRunbookAlertNameLength)}
		}
		seen[strings.ToLower(alert)] = true
		out.Alerts = append(out.Alerts, alert)
	}
	if len(out.Alerts) == 0 {
		return req, &apierror.ValidationError{Msg: "alerts must have at least one alert name"}
	}
	if len(out.Alerts) > maxRunbookAlerts {
		return req, &apierror.ValidationError{Msg: fmt.Sprintf("alerts must have at most %d alert names", maxRunbookAlerts)}
	}
	if req.RunbookURL != nil {
		link := strings.TrimSpace(*req.RunbookURL)
		if link != "" {
			if err := validateRunbookURL(link); err != nil {
				return req, err
			}
			out.RunbookURL = &link
		}
	}
	return out, nil
}

func validateRunbookURL(link string) error {
	if len(link) > maxRunbookURLLength {
		return &apierror.ValidationError{Msg: fmt.Sprintf("runbookUrl must be at most %d characters", maxRunbookURLLength)}
	}
	u, err := url.Parse(link)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return &apierror.ValidationError{Msg: "runbookUrl must be a full http(s) link"}
	}
	return nil
}
