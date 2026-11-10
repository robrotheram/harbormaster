package api

import (
	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/shared"
	"database/sql"
	"errors"
	"net/http"
)

func workError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
	case errors.Is(err, orchestration.ErrConflict):
		shared.Error(w, 409, "WORK_STATE_CONFLICT")
	case errors.Is(err, orchestration.ErrInvalidWork):
		shared.Error(w, 400, "INVALID_REQUEST")
	default:
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
	}
}
func (s *Server) registerWorkExecution(mux *http.ServeMux, repo orchestration.Repository) {
	mux.HandleFunc("POST /v1/work-items/{id}/run", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ProjectID string `json:"project_id"`
			Key       string `json:"idempotency_key"`
		}
		if !shared.Decode(w, r, &in, 2048) || !s.AuthorizeProject(w, r, in.ProjectID) {
			return
		}
		unlock := s.operations.Lock("work:" + r.PathValue("id"))
		defer unlock()
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
			ProjectID string `json:"project_id"`
			Accepted  *bool  `json:"accepted"`
		}
		if !shared.Decode(w, r, &in, 1024) || !s.AuthorizeProject(w, r, in.ProjectID) {
			return
		}
		if in.Accepted == nil {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		unlock := s.operations.Lock("work:" + r.PathValue("id"))
		defer unlock()
		item, err := repo.ReviewWork(r.Context(), in.ProjectID, r.PathValue("id"), *in.Accepted, s.now())
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
				if err != nil || ws.ProjectID != project {
					shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
					return
				}
				unlock := s.operations.Lock(ws.ID)
				defer unlock()
				if err := s.lifecycle().Start(r.Context(), &ws); err != nil {
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
				w.Header().Set("Cache-Control", "private, no-store")
				w.Header().Set("X-Content-Type-Options", "nosniff")
				w.Write(b)
				return
			}
		}
		shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
	})
}
