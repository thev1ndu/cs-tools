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
	"html"
	"log/slog"
	"strings"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
)

// runbookLister is the part of the runbook catalog a new incident's work note needs.
type runbookLister interface {
	List(ctx context.Context) ([]domain.Runbook, error)
}

const (
	noRunbookNoteText = "No runbook available for this alert."
	// The table styles match sre-alert-core-service's incident creation note, so the two notes read alike.
	runbookNoteTableStyle  = "border:1px solid #dcdcdc; border-collapse:collapse; width:100%; font-family:Arial, sans-serif; font-size:13px;"
	runbookNoteHeaderStyle = "background:#f5f5f5; font-weight:bold; text-align:left; padding:8px; border:1px solid #dcdcdc;"
	runbookNoteCellStyle   = "padding:8px; border:1px solid #dcdcdc;"
)

// matchRunbook is the entry whose alert name the subject contains, ignoring case; the longest name wins.
func matchRunbook(runbooks []domain.Runbook, subject string) *domain.Runbook {
	haystack := strings.ToLower(subject)
	var best *domain.Runbook
	bestLen := 0
	for i := range runbooks {
		needle := strings.ToLower(strings.TrimSpace(runbooks[i].AlertName))
		if needle == "" || len(needle) <= bestLen || !strings.Contains(haystack, needle) {
			continue
		}
		best = &runbooks[i]
		bestLen = len(needle)
	}
	return best
}

// runbookNote is the HTML work note for a new incident, or "" when it gets none: a catalog match, or an alert-born incident with no match.
func runbookNote(runbooks []domain.Runbook, req domain.CreateIncidentRequest) string {
	if rb := matchRunbook(runbooks, req.Subject); rb != nil {
		runbook := noRunbookNoteText
		if rb.RunbookURL != nil && strings.TrimSpace(*rb.RunbookURL) != "" {
			link := html.EscapeString(strings.TrimSpace(*rb.RunbookURL))
			runbook = `<a href="` + link + `" target="_blank" rel="noopener noreferrer">` + link + `</a>`
		}
		return runbookNoteTable(html.EscapeString(rb.AlertName), runbook)
	}
	if req.CorrelationID != nil && strings.TrimSpace(*req.CorrelationID) != "" {
		return "<p>" + noRunbookNoteText + "</p>"
	}
	return ""
}

// runbookNoteTable renders the already-escaped alert name and runbook cell under an Alert | Runbook header row.
func runbookNoteTable(alert, runbook string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<table style='%s'>", runbookNoteTableStyle)
	fmt.Fprintf(&b, "<tr><th style='%s'>Alert</th><th style='%s'>Runbook</th></tr>", runbookNoteHeaderStyle, runbookNoteHeaderStyle)
	fmt.Fprintf(&b, "<tr><td style='%s'>%s</td><td style='%s'>%s</td></tr>", runbookNoteCellStyle, alert, runbookNoteCellStyle, runbook)
	b.WriteString("</table>")
	return b.String()
}

// withRunbookNote appends the runbook work note right after the caller's own; a catalog read failure is logged and leaves the request as it was.
func withRunbookNote(ctx context.Context, req domain.CreateIncidentRequest, catalog runbookLister) domain.CreateIncidentRequest {
	if catalog == nil {
		return req
	}
	runbooks, err := catalog.List(ctx)
	if err != nil {
		slog.WarnContext(ctx, "incident create: runbook catalog unavailable; creating the incident without a runbook note", "error", err)
		return req
	}
	note := runbookNote(runbooks, req)
	if note == "" {
		return req
	}
	// Joined with no line break: the note starts with its own block element, and comments render source line breaks as gaps.
	if req.WorkNotes != nil && strings.TrimSpace(*req.WorkNotes) != "" {
		note = *req.WorkNotes + note
	}
	req.WorkNotes = &note
	return req
}

// WithRunbookCatalog sets the runbook catalog new incidents are matched against; applies to the Postgres and the ServiceNow incident services.
func WithRunbookCatalog(svc IncidentService, catalog runbookLister) IncidentService {
	switch s := svc.(type) {
	case *incidentService:
		s.runbooks = catalog
	case *snIncidentService:
		s.runbooks = catalog
	}
	return svc
}
