package api

import (
	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/shared"
	"database/sql"
	"errors"
	"net/http"
)

func workError(w http.ResponseWriter, err error) {
	var response error
	switch {
	case errors.Is(err, sql.ErrNoRows):
		response = shared.CodedError{Status: http.StatusNotFound, Code: "NOT_FOUND_OR_FORBIDDEN"}
	case errors.Is(err, orchestration.ErrConflict):
		response = shared.CodedError{Status: http.StatusConflict, Code: "WORK_STATE_CONFLICT"}
	case errors.Is(err, orchestration.ErrInvalidWork):
		response = orchestration.ErrInvalidWork
	}
	shared.WriteHTTPError(w, response, storageUnavailable)
}
func (s *Server) registerWorkExecution(mux *http.ServeMux, repo orchestration.Repository) {
	s.registerWorkImages(mux, repo)
	mux.HandleFunc("POST /v1/work-items/{id}/run", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ProjectID string `json:"project_id"`
			Key       string `json:"idempotency_key"`
		}
		if !decodeRouteBody(w, r, &in) || !s.AuthorizeProject(w, r, in.ProjectID) {
			return
		}
		unlock := s.operations.Lock("work:" + r.PathValue("id"))
		defer unlock()
		if !s.authorizeWorkController(w, r, in.ProjectID, r.PathValue("id"), repo) {
			return
		}
		a, err := repo.RunWork(r.Context(), in.ProjectID, r.PathValue("id"), in.Key, s.now())
		if err != nil {
			workError(w, err)
			return
		}
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
	mux.HandleFunc("POST /v1/work-items/{id}/review", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ProjectID         string `json:"project_id"`
			Accepted          *bool  `json:"accepted"`
			Decision          string `json:"decision,omitempty"`
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
		item, err := repo.ReviewRevisionBy(r.Context(), in.ProjectID, r.PathValue("id"), decision, in.ExpectedAttemptID, actor.human.ID, s.now())
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
