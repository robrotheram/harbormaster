package api

import (
	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/shared"
	"database/sql"
	"errors"
	"net/http"
)

func (s *Server) registerWork(mux *http.ServeMux) {
	repo := orchestration.Repository{DB: s.Store.DB}
	mux.HandleFunc("GET /v1/work-items", func(w http.ResponseWriter, r *http.Request) {
		project, ok := s.ProjectForRequest(w, r, r.URL.Query().Get("project_id"))
		if !ok {
			return
		}
		if project == "" {
			shared.Error(w, 400, "PROJECT_REQUIRED")
			return
		}
		items, err := repo.WorkItems(r.Context(), project)
		if err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, map[string]any{"items": items})
	})
	mux.HandleFunc("GET /v1/work-items/{id}", func(w http.ResponseWriter, r *http.Request) {
		project, ok := s.ProjectForRequest(w, r, r.URL.Query().Get("project_id"))
		if !ok {
			return
		}
		if project == "" || !shared.ValidID(r.PathValue("id")) {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		item, err := repo.Work(r.Context(), project, r.PathValue("id"))
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			} else {
				shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			}
			return
		}
		shared.JSON(w, 200, item)
	})
	mux.HandleFunc("POST /v1/work-items/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			ProjectID string `json:"project_id"`
		}
		if !shared.Decode(w, r, &input, 1024) || !s.AuthorizeProject(w, r, input.ProjectID) {
			return
		}
		if !shared.ValidID(r.PathValue("id")) {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		item, err := repo.CancelDraft(r.Context(), input.ProjectID, r.PathValue("id"), s.now())
		if errors.Is(err, orchestration.ErrConflict) {
			shared.Error(w, 409, "WORK_STATE_CONFLICT")
			return
		}
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			} else {
				shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			}
			return
		}
		shared.JSON(w, 200, item)
	})

	mux.HandleFunc("POST /v1/work-items", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			ProjectID   string `json:"project_id"`
			WorkspaceID string `json:"workspace_id"`
			Path        string `json:"specification_path"`
			Digest      string `json:"specification_digest"`
			Kind        string `json:"kind"`
			Key         string `json:"idempotency_key"`
		}
		if !shared.Decode(w, r, &input, 4096) || !s.AuthorizeProject(w, r, input.ProjectID) {
			return
		}
		ws, err := s.Store.Workspace(r.Context(), input.WorkspaceID)
		if err != nil || ws.ProjectID != input.ProjectID {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		item, created, err := repo.CreateWork(r.Context(), shared.WorkItem{ProjectID: input.ProjectID, WorkspaceID: input.WorkspaceID, SpecificationPath: input.Path, SpecificationDigest: input.Digest, Kind: input.Kind, CreatedAt: s.now()}, input.Key)
		if errors.Is(err, orchestration.ErrConflict) {
			shared.Error(w, 409, "IDEMPOTENCY_CONFLICT")
			return
		}
		if err != nil {
			if errors.Is(err, orchestration.ErrInvalidWork) {
				shared.Error(w, 400, "INVALID_REQUEST")
			} else {
				shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			}
			return
		}
		status := 200
		if created {
			status = 201
		}
		shared.JSON(w, status, item)
	})
}
