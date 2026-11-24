package api

import (
	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/shared"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

func workError(w http.ResponseWriter, err error) {
	var response error
	switch {
	case errors.Is(err, sql.ErrNoRows):
		response = shared.CodedError{Status: http.StatusNotFound, Code: "NOT_FOUND_OR_FORBIDDEN"}
	case errors.Is(err, orchestration.ErrConflict):
		response = shared.CodedError{Status: http.StatusConflict, Code: "WORK_STATE_CONFLICT"}
	case errors.Is(err, subscriptionRequired):
		response = subscriptionRequired
	case errors.Is(err, agentSelectionRequired):
		response = agentSelectionRequired
	case errors.Is(err, orchestration.ErrInvalidWork):
		response = orchestration.ErrInvalidWork
	}
	shared.WriteHTTPError(w, response, storageUnavailable)
}
func (s *Server) registerWorkExecution(mux *http.ServeMux, repo orchestration.Repository) {
	s.registerWorkImages(mux, repo)
	s.registerWorkFiles(mux, repo)
	s.registerWorkReports(mux, repo)
	mux.HandleFunc("POST /v1/work-items/{id}/run", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ProjectID string             `json:"project_id"`
			Key       string             `json:"idempotency_key"`
			Selection *shared.AgentModel `json:"selection,omitempty"`
		}
		if !decodeRouteBody(w, r, &in) || !s.AuthorizeProject(w, r, in.ProjectID) {
			return
		}
		unlock := s.operations.Lock("work:" + r.PathValue("id"))
		defer unlock()
		if !s.authorizeWorkController(w, r, in.ProjectID, r.PathValue("id"), repo) {
			return
		}
		if !shared.ValidKey(in.Key) || !validExplicitSelection(in.Selection) {
			workError(w, orchestration.ErrInvalidWork)
			return
		}
		id := r.PathValue("id")
		if existing, err := repo.AttemptByDeliveryKey(r.Context(), in.ProjectID, id, in.Key); err == nil {
			if in.Selection != nil && (existing.Provider != in.Selection.Provider || existing.Model != in.Selection.Model || existing.ReasoningEffort != in.Selection.ReasoningEffort) {
				workError(w, orchestration.ErrConflict)
				return
			}
			shared.JSON(w, 202, existing)
			return
		} else if !errors.Is(err, sql.ErrNoRows) {
			workError(w, err)
			return
		}
		item, err := repo.Work(r.Context(), in.ProjectID, id)
		if err != nil {
			workError(w, err)
			return
		}
		card, err := s.Store.BoardItem(r.Context(), in.ProjectID, item.ID)
		if err != nil {
			workError(w, err)
			return
		}
		if !s.checkWorkDependencies(w, r, repo, card) {
			return
		}
		provider := item.Provider
		if in.Selection != nil {
			provider = in.Selection.Provider
		}
		if err = s.prepareWorkSubscription(r, item.WorkspaceID, provider); err != nil {
			workError(w, err)
			return
		}
		if in.Selection != nil {
			release := s.operations.Lock(item.WorkspaceID)
			defer release()
			ws, err := s.Store.Workspace(r.Context(), item.WorkspaceID)
			if err != nil || !s.workspaceInProject(ws, in.ProjectID) {
				shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
				return
			}
			if err := s.start(r.Context(), &ws); err != nil {
				apiError(w, err)
				return
			}
			if err := s.checkAgentSelection(r.Context(), ws.ID, in.Selection); err != nil {
				apiError(w, err)
				return
			}
		}
		a, err := repo.RunWorkWithSelection(r.Context(), in.ProjectID, id, in.Key, in.Selection, s.now())
		if err != nil {
			workError(w, err)
			return
		}
		s.notifyWorkReady()
		shared.JSON(w, 202, a)
	})
	mux.HandleFunc("GET /v1/work-items/{id}/attempts", func(w http.ResponseWriter, r *http.Request) {
		project, ok := s.ProjectForRequest(w, r, r.URL.Query().Get("project_id"))
		if !ok {
			return
		}
		if project == "" {
			shared.Error(w, 400, "PROJECT_REQUIRED")
			return
		}
		items, err := repo.Attempts(r.Context(), project, r.PathValue("id"))
		if err != nil {
			workError(w, err)
			return
		}
		shared.JSON(w, 200, map[string]any{"items": items})
	})
	mux.HandleFunc("GET /v1/work-items/{id}/attempts/{attempt}/changes", func(w http.ResponseWriter, r *http.Request) {
		project, ok := s.ProjectForRequest(w, r, r.URL.Query().Get("project_id"))
		if !ok {
			return
		}
		if project == "" {
			shared.Error(w, 400, "PROJECT_REQUIRED")
			return
		}
		ctx := r.Context()
		attempt, err := repo.Attempt(ctx, project, r.PathValue("id"), r.PathValue("attempt"))
		if err != nil {
			workError(w, err)
			return
		}
		var changes *shared.TaskChanges
		if attempt.TaskID == "" || shared.Active(attempt.State) {
			shared.JSON(w, 200, changes)
			return
		}
		ws, err := s.Store.Workspace(ctx, attempt.WorkspaceID)
		if err != nil || !s.workspaceInProject(ws, project) {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		unlock := s.operations.Lock(ws.ID)
		defer unlock()
		if err := s.lifecycle().ObserveTask(ctx, &ws); err != nil {
			shared.Error(w, 503, "WORKSPACE_UNAVAILABLE")
			return
		}
		status, data, err := s.Runtime.Agent(ctx, ws.ID, "GET", "/v1/tasks/"+attempt.TaskID+"/changes", nil)
		if err != nil {
			shared.Error(w, 503, "WORKSPACE_UNAVAILABLE")
			return
		}
		if status != 404 && (status != 200 || json.Unmarshal(data, &changes) != nil || changes == nil || changes.Files == nil) {
			shared.Error(w, 503, "TASK_CHANGES_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, changes)
	})
	mux.HandleFunc("POST /v1/work-items/{id}/review", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ProjectID         string `json:"project_id"`
			Accepted          *bool  `json:"accepted"`
			Decision          string `json:"decision,omitempty"`
			Feedback          string `json:"feedback,omitempty"`
			ExpectedAttemptID string `json:"expected_attempt_id"`
		}
		if !decodeRouteBody(w, r, &in) || !s.AuthorizeProject(w, r, in.ProjectID) {
			return
		}
		if (in.Decision != "" && in.Decision != "closed") || (in.Decision == "closed" && in.Accepted != nil) || (in.Decision == "" && in.Accepted == nil) {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		unlock := s.operations.Lock("work:" + r.PathValue("id"))
		defer unlock()
		current, err := repo.Work(r.Context(), in.ProjectID, r.PathValue("id"))
		if err != nil {
			workError(w, err)
			return
		}
		if current.State != "awaiting_review" && in.ExpectedAttemptID == "" {
			shared.Error(w, 409, "WORK_STATE_CONFLICT")
			return
		}
		if (current.State == "awaiting_review" || in.ExpectedAttemptID != "") && (in.ExpectedAttemptID == "" || current.LatestAttemptID != in.ExpectedAttemptID) {
			shared.Error(w, 409, "WORK_STATE_CONFLICT")
			return
		}
		actor, _ := r.Context().Value(principalKey{}).(principal)
		if current.ReviewerID != "" && !actor.admin && actor.human.ID != current.ReviewerID {
			shared.Error(w, 403, "ASSIGNED_REVIEWER_REQUIRED")
			return
		}
		decision := "changes_requested"
		if in.Decision == "closed" {
			decision = "closed"
		} else if *in.Accepted {
			decision = "accepted"
		}
		feedback := strings.TrimSpace(in.Feedback)
		if decision == "changes_requested" && (feedback == "" || len(feedback) > 4000) {
			shared.Error(w, 400, "REVIEW_FEEDBACK_REQUIRED")
			return
		}
		if decision != "changes_requested" {
			feedback = ""
		}
		item, err := repo.ReviewRevisionBy(r.Context(), in.ProjectID, r.PathValue("id"), decision, in.ExpectedAttemptID, actor.human.ID, feedback, s.now())
		if err != nil {
			workError(w, err)
			return
		}
		shared.JSON(w, 200, item)
	})
	mux.HandleFunc("GET /v1/work-items/{id}/artifacts/{artifact}/content", func(w http.ResponseWriter, r *http.Request) {
		project, ok := s.ProjectForRequest(w, r, r.URL.Query().Get("project_id"))
		if !ok {
			return
		}
		if project == "" {
			shared.Error(w, 400, "PROJECT_REQUIRED")
			return
		}
		attempts, err := repo.Attempts(r.Context(), project, r.PathValue("id"))
		if err != nil {
			workError(w, err)
			return
		}
		for _, a := range attempts {
			for _, artifact := range a.Artifacts {
				if artifact.ID != r.PathValue("artifact") || !shared.ValidID(artifact.ID) {
					continue
				}
				ws, err := s.Store.Workspace(r.Context(), a.WorkspaceID)
				if err != nil || !s.workspaceInProject(ws, project) {
					shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
					return
				}
				unlock := s.operations.Lock(ws.ID)
				defer unlock()
				if err := s.lifecycle().ObserveTask(r.Context(), &ws); err != nil {
					shared.Error(w, 503, "WORKSPACE_UNAVAILABLE")
					return
				}
				status, b, err := s.Runtime.Agent(r.Context(), ws.ID, "GET", "/v1/artifacts/"+artifact.ID+"/content", nil)
				if err != nil {
					shared.Error(w, 503, "WORKSPACE_UNAVAILABLE")
					return
				}
				if status != 200 {
					shared.Error(w, 503, "ARTIFACT_UNAVAILABLE")
					return
				}
				digest := sha256.Sum256(b)
				if artifact.SHA256 != hex.EncodeToString(digest[:]) {
					shared.Error(w, http.StatusBadGateway, "ARTIFACT_DIGEST_MISMATCH")
					return
				}
				w.Header().Set("Content-Type", "application/octet-stream")
				w.Header().Set("Content-Disposition", "attachment")
				w.Header().Set("Cache-Control", "private, no-store")
				w.Header().Set("X-Content-Type-Options", "nosniff")
				w.Write(b)
				return
			}
		}
		shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
	})
}
