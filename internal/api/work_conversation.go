package api

import (
	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/shared"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Conversation bodies stay in the worker's private files and history. SQL holds
// only immutable references, request fingerprints and execution receipts.
func (s *Server) registerWorkConversation(mux *http.ServeMux, repo orchestration.Repository) {
	mux.HandleFunc("GET /v1/work-items/{id}/instruction", func(w http.ResponseWriter, r *http.Request) {
		project, ok := s.ProjectForRequest(w, r, r.URL.Query().Get("project_id"))
		if !ok {
			return
		}
		if project == "" {
			shared.Error(w, 400, "PROJECT_REQUIRED")
			return
		}
		item, err := repo.Work(r.Context(), project, r.PathValue("id"))
		if err != nil {
			workError(w, err)
			return
		}
		ws, err := s.Store.Workspace(r.Context(), item.WorkspaceID)
		if err != nil || !s.workspaceInProject(ws, project) {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		if err := s.refresh(r.Context(), &ws); err != nil {
			shared.Error(w, 503, "HISTORY_UNAVAILABLE")
			return
		}
		if !ws.Ready {
			shared.Error(w, 409, "HISTORY_WORKSPACE_STOPPED")
			return
		}
		body, _ := json.Marshal(shared.FileHandoff{Path: item.SpecificationPath})
		status, data, err := s.Runtime.Agent(r.Context(), ws.ID, "POST", "/v1/files/read", body)
		var file shared.FileHandoff
		if err != nil || status != 200 || json.Unmarshal(data, &file) != nil || len(file.Content) > 65536 {
			shared.Error(w, 503, "INSTRUCTION_UNAVAILABLE")
			return
		}
		digest := sha256.Sum256(file.Content)
		if hex.EncodeToString(digest[:]) != item.SpecificationDigest {
			shared.Error(w, 409, "SPECIFICATION_DIGEST_MISMATCH")
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		shared.JSON(w, 200, map[string]string{"project_id": project, "work_item_id": item.ID, "instruction": string(file.Content)})
	})
	mux.HandleFunc("POST /v1/work-items/{id}/continue", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ProjectID         string `json:"project_id"`
			Instruction       string `json:"instruction"`
			Presentation      string `json:"presentation,omitempty"`
			ExpectedAttemptID string `json:"expected_attempt_id"`
			Key               string `json:"idempotency_key"`
		}
		if !shared.Decode(w, r, &in, 100000) || !s.AuthorizeProject(w, r, in.ProjectID) {
			return
		}
		if (in.Presentation != "" && in.Presentation != "interactive-v1") || !shared.ValidID(in.ProjectID) || !shared.ValidID(in.ExpectedAttemptID) || !shared.ValidKey(in.Key) || strings.TrimSpace(in.Instruction) == "" || len(in.Instruction) > 16000 || !utf8.ValidString(in.Instruction) {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		item, err := repo.Work(r.Context(), in.ProjectID, r.PathValue("id"))
		if err != nil {
			workError(w, err)
			return
		}
		unlock := s.operations.Lock("work:"+item.ID, item.WorkspaceID)
		defer unlock()
		item, err = repo.Work(r.Context(), in.ProjectID, item.ID)
		if err != nil {
			workError(w, err)
			return
		}
		identity := sha256.Sum256([]byte(item.ID + "\x00" + in.Key))
		key := "followup-" + hex.EncodeToString(identity[:])
		path := "work-requests/" + key + ".md"
		bytes, _ := json.Marshal(in)
		fingerprint := sha256.Sum256(bytes)
		if err = repo.ClaimWorkRequest(r.Context(), in.ProjectID, key, hex.EncodeToString(fingerprint[:])); err != nil {
			if err == orchestration.ErrConflict {
				shared.Error(w, 409, "IDEMPOTENCY_CONFLICT")
			} else {
				workError(w, err)
			}
			return
		}
		attempts, err := repo.Attempts(r.Context(), in.ProjectID, item.ID)
		if err != nil {
			workError(w, err)
			return
		}
		// Resolve an accepted retry before inspecting activity or publishing files.
		for _, a := range attempts {
			if a.SpecificationPath == path {
				shared.JSON(w, 200, a)
				return
			}
		}
		if item.LatestAttemptID != in.ExpectedAttemptID {
			shared.Error(w, 409, "WORK_STATE_CONFLICT")
			return
		}
		switch item.State {
		case "planned", "in_progress", "completed", "awaiting_review", "failed", "cancelled":
		default:
			shared.Error(w, 409, "WORK_STATE_CONFLICT")
			return
		}
		ws, err := s.Store.Workspace(r.Context(), item.WorkspaceID)
		if err != nil || !s.workspaceInProject(ws, in.ProjectID) {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		if err = s.start(r.Context(), &ws); err != nil {
			apiError(w, err)
			return
		}
		// This is a new native run with explicit file context, not an unsupported
		// native session resume or injection into an executing task.
		presentation := ""
		if in.Presentation == "interactive-v1" {
			presentation = "\n\n" + visualizationGuidance
		}
		instruction := "Continue the same work in this workspace. Read the previous request at " + strconv.Quote(item.SpecificationPath) + " for context and inspect the existing files. Apply the current request without repeating completed changes. Run relevant checks and update the requested output files." + presentation + "\n\nCurrent request:\n" + in.Instruction
		if len(instruction) > 32768 {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		digest := sha256.Sum256([]byte(instruction))
		hexDigest := hex.EncodeToString(digest[:])
		body, _ := json.Marshal(shared.FileHandoff{Path: path, Content: []byte(instruction)})
		status, reply, err := s.Runtime.Agent(r.Context(), ws.ID, "POST", "/v1/files/write-new", body)
		if err != nil {
			shared.Error(w, 503, "WORKSPACE_UNAVAILABLE")
			return
		}
		if status != 201 {
			var failure shared.APIError
			_ = json.Unmarshal(reply, &failure)
			if status == 409 && failure.Code == "WORKSPACE_BUSY" {
				shared.Error(w, 409, "WORKSPACE_BUSY")
				return
			}
			if status != 409 || failure.Code != "DESTINATION_EXISTS" {
				shared.Error(w, 503, "PRIVATE_PUBLICATION_FAILED")
				return
			}
			body, _ = json.Marshal(shared.FileHandoff{Path: path})
			status, reply, err = s.Runtime.Agent(r.Context(), ws.ID, "POST", "/v1/files/read", body)
			var prior shared.FileHandoff
			if err != nil || status != 200 || json.Unmarshal(reply, &prior) != nil {
				shared.Error(w, 503, "PRIVATE_PUBLICATION_UNCONFIRMED")
				return
			}
			if sha256.Sum256(prior.Content) != digest {
				shared.Error(w, 409, "IDEMPOTENCY_CONFLICT")
				return
			}
		}
		a, err := repo.ContinueWork(r.Context(), in.ProjectID, item.ID, key, orchestration.WorkRevision{ExpectedAttemptID: in.ExpectedAttemptID, Path: path, Digest: hexDigest}, s.now())
		if err != nil {
			workError(w, err)
			return
		}
		shared.JSON(w, 202, a)
	})
}
