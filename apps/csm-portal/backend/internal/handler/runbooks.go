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
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/middleware"
)

const (
	// maxRunbookImportEntries mirrors entity-service's own cap so an oversized import is refused here first.
	maxRunbookImportEntries = 5000
	// maxRunbookImportBodyBytes matches entity-service's import limit: 5000 single-alert entries with the longest name, alert name and link fit.
	maxRunbookImportBodyBytes = 32 << 20
)

// entityRunbookClient is the entity-service calls the Runbooks tab needs.
type entityRunbookClient interface {
	ListRunbooks(ctx context.Context) ([]byte, error)
	CreateRunbook(ctx context.Context, body []byte) ([]byte, error)
	UpdateRunbook(ctx context.Context, id string, body []byte) ([]byte, error)
	DeleteRunbook(ctx context.Context, id string) ([]byte, error)
	ImportRunbooks(ctx context.Context, body []byte) ([]byte, error)
}

// RunbookHandler proxies the runbook catalog (Operations > Runbooks); entity-service validates the values.
type RunbookHandler struct {
	entity entityRunbookClient
}

// NewRunbookHandler creates a RunbookHandler.
func NewRunbookHandler(entity entityRunbookClient) *RunbookHandler {
	return &RunbookHandler{entity: entity}
}

type runbookBody struct {
	Name       string   `json:"name"`
	RunbookURL *string  `json:"runbookUrl"`
	Alerts     []string `json:"alerts"`
}

// complete reports whether the runbook has a name and at least one alert name.
func (b runbookBody) complete() bool {
	if strings.TrimSpace(b.Name) == "" {
		return false
	}
	for _, a := range b.Alerts {
		if strings.TrimSpace(a) != "" {
			return true
		}
	}
	return false
}

// ListRunbooks handles GET /runbooks.
func (h *RunbookHandler) ListRunbooks(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}
	h.forward(w, r, user.UserID, "ListRunbooks", "Failed to load runbooks.", http.StatusOK, h.entity.ListRunbooks)
}

// CreateRunbook handles POST /runbooks.
func (h *RunbookHandler) CreateRunbook(w http.ResponseWriter, r *http.Request) {
	body, callerID, ok := readRunbookBody(w, r)
	if !ok {
		return
	}
	h.forward(w, r, callerID, "CreateRunbook", "Failed to add the runbook.", http.StatusCreated,
		func(ctx context.Context) ([]byte, error) { return h.entity.CreateRunbook(ctx, body) })
}

// UpdateRunbook handles PUT /runbooks/{id}.
func (h *RunbookHandler) UpdateRunbook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}
	body, callerID, ok := readRunbookBody(w, r)
	if !ok {
		return
	}
	h.forward(w, r, callerID, "UpdateRunbook", "Failed to save the runbook.", http.StatusOK,
		func(ctx context.Context) ([]byte, error) { return h.entity.UpdateRunbook(ctx, id, body) })
}

// DeleteRunbook handles DELETE /runbooks/{id}.
func (h *RunbookHandler) DeleteRunbook(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}
	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}
	h.forward(w, r, user.UserID, "DeleteRunbook", "Failed to delete the runbook.", http.StatusNoContent,
		func(ctx context.Context) ([]byte, error) { return h.entity.DeleteRunbook(ctx, id) })
}

// ImportRunbooks handles POST /runbooks/import.
func (h *RunbookHandler) ImportRunbooks(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}
	callerID := user.UserID
	r.Body = http.MaxBytesReader(w, r.Body, maxRunbookImportBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, ErrMsgTooLarge)
			return
		}
		writeError(w, http.StatusBadRequest, errMsgReadBody)
		return
	}
	var req struct {
		Runbooks []runbookBody `json:"runbooks"`
	}
	if err := json.Unmarshal(body, &req); err != nil || len(req.Runbooks) == 0 || len(req.Runbooks) > maxRunbookImportEntries {
		writeError(w, http.StatusBadRequest, "Send between 1 and 5000 runbooks.")
		return
	}
	for _, rb := range req.Runbooks {
		if !rb.complete() {
			writeError(w, http.StatusBadRequest, "Every runbook needs a name and at least one alert name.")
			return
		}
	}
	fwd := mustJSON(map[string]any{"runbooks": req.Runbooks})
	h.forward(w, r, callerID, "ImportRunbooks", "Failed to import runbooks.", http.StatusOK,
		func(ctx context.Context) ([]byte, error) { return h.entity.ImportRunbooks(ctx, fwd) })
}

// readRunbookBody reads one entry and rebuilds it from the known fields.
func readRunbookBody(w http.ResponseWriter, r *http.Request) ([]byte, string, bool) {
	body, callerID, ok := readScheduleBody(w, r)
	if !ok {
		return nil, "", false
	}
	var req runbookBody
	if err := json.Unmarshal(body, &req); err != nil || !req.complete() {
		writeError(w, http.StatusBadRequest, "Send a name, at least one alert name and, optionally, a runbook link.")
		return nil, "", false
	}
	return mustJSON(req), callerID, true
}

// forward makes the call; a refusal keeps entity-service's own reason so the tab can show it.
func (h *RunbookHandler) forward(w http.ResponseWriter, r *http.Request, callerID, op, fallbackMsg string, okStatus int,
	call func(ctx context.Context) ([]byte, error)) {
	result, err := call(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "entity "+op+" failed", "userID", callerID, "err", err)
		var apiErr *apierror.Error
		if errors.As(err, &apiErr) {
			switch apiErr.StatusCode {
			case http.StatusBadRequest, http.StatusNotFound, http.StatusConflict:
				writeError(w, apiErr.StatusCode, upstreamErrorMessageStrict(apiErr.Body, fallbackMsg))
				return
			}
		}
		mapUpstreamErrorGeneric(w, err, fallbackMsg)
		return
	}
	if okStatus == http.StatusNoContent {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, okStatus, result)
}
