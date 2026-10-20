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
	"encoding/json"
	"net/http"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/service"
)

// RunbookHandler serves the runbook catalog (Operations > Runbooks).
type RunbookHandler struct {
	svc service.RunbookService
}

// NewRunbookHandler constructs a RunbookHandler.
func NewRunbookHandler(svc service.RunbookService) *RunbookHandler {
	return &RunbookHandler{svc: svc}
}

// maxRunbookImportBodySize fits the largest valid import: 5000 entries of a 1000-character alert name and a 2000-character link.
const maxRunbookImportBodySize = 32 << 20

func writeRunbookJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// ListRunbooks handles GET /runbooks.
func (h *RunbookHandler) ListRunbooks(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.List(r.Context())
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeRunbookJSON(w, http.StatusOK, resp)
}

// CreateRunbook handles POST /runbooks.
func (h *RunbookHandler) CreateRunbook(w http.ResponseWriter, r *http.Request) {
	var req domain.SaveRunbookRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	rb, err := h.svc.Create(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeRunbookJSON(w, http.StatusCreated, rb)
}

// UpdateRunbook handles PUT /runbooks/{id}.
func (h *RunbookHandler) UpdateRunbook(w http.ResponseWriter, r *http.Request) {
	var req domain.SaveRunbookRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	rb, err := h.svc.Update(r.Context(), r.PathValue("id"), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeRunbookJSON(w, http.StatusOK, rb)
}

// DeleteRunbook handles DELETE /runbooks/{id}.
func (h *RunbookHandler) DeleteRunbook(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ImportRunbooks handles POST /runbooks/import.
func (h *RunbookHandler) ImportRunbooks(w http.ResponseWriter, r *http.Request) {
	var req domain.ImportRunbooksRequest
	if !decodeRequestWithLimit(w, r, &req, maxRunbookImportBodySize, "import too large: send at most 32 MiB") {
		return
	}
	resp, err := h.svc.Import(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeRunbookJSON(w, http.StatusOK, resp)
}

// MergeRunbooks handles POST /runbooks/merge.
func (h *RunbookHandler) MergeRunbooks(w http.ResponseWriter, r *http.Request) {
	var req domain.MergeRunbooksRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	rb, err := h.svc.Merge(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeRunbookJSON(w, http.StatusOK, rb)
}

// UnmergeRunbookAlerts handles POST /runbooks/{id}/unmerge.
func (h *RunbookHandler) UnmergeRunbookAlerts(w http.ResponseWriter, r *http.Request) {
	var req domain.UnmergeRunbookAlertsRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	resp, err := h.svc.Unmerge(r.Context(), r.PathValue("id"), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeRunbookJSON(w, http.StatusOK, resp)
}
