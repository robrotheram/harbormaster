package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/shared"
)

func (s *Server) registerOrchestration(mux *http.ServeMux) {
	repo := orchestration.Repository{DB: s.Store.DB}
	mux.HandleFunc("GET /v1/schedules", func(w http.ResponseWriter, r *http.Request) {
		project, ok := s.ProjectForRequest(w, r, r.URL.Query().Get("project_id"))
		if !ok {
			return
		}
		items, err := repo.Schedules(r.Context(), project)
		if err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, map[string]any{"items": items})
	})
	mux.HandleFunc("GET /v1/runs", func(w http.ResponseWriter, r *http.Request) {
		project, ok := s.ProjectForRequest(w, r, r.URL.Query().Get("project_id"))
		if !ok {
			return
		}
		items, err := repo.Runs(r.Context(), project)
		if err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, map[string]any{"items": items})
	})
	mux.HandleFunc("POST /v1/schedules", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			ProjectID       string    `json:"project_id"`
			WorkspaceID     string    `json:"workspace_id"`
			Name            string    `json:"name"`
			InstructionPath string    `json:"instruction_path"`
			Provider        string    `json:"provider"`
			RunAt           time.Time `json:"run_at"`
			Interval        int64     `json:"interval_seconds"`
			Key             string    `json:"idempotency_key"`
		}
		if !shared.Decode(w, r, &input, 4096) {
			return
		}
		if !s.AuthorizeProject(w, r, input.ProjectID) {
			return
		}
		input.Name = strings.TrimSpace(input.Name)
		if input.Provider == "" {
			input.Provider = "codex"
		}
		if !shared.ValidID(input.WorkspaceID) || !shared.ValidKey(input.Key) || input.Name == "" || len(input.Name) > 80 || !shared.ValidProjectFile(input.InstructionPath) || (input.Provider != "codex" && input.Provider != "hermes") || input.RunAt.IsZero() || input.Interval < 0 || (input.Interval > 0 && input.Interval < 60) || input.Interval > 31536000 {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		unlock := s.operations.Lock("schedule-create:" + input.ProjectID)
		defer unlock()
		ws, err := s.Store.Workspace(r.Context(), input.WorkspaceID)
		if err != nil || ws.ProjectID != input.ProjectID {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		if input.Provider == "hermes" && ws.AgentKind != "hermes" {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		now := s.now()
		schedule := shared.Schedule{ID: shared.NewID("schedule_"), ProjectID: input.ProjectID, WorkspaceID: input.WorkspaceID, Name: input.Name, InstructionPath: input.InstructionPath, Provider: input.Provider, Enabled: true, RunAt: input.RunAt.UTC(), IntervalSeconds: input.Interval, NextRunAt: input.RunAt.UTC(), CreatedAt: now}
		stored, created, err := repo.CreateSchedule(r.Context(), schedule, input.Key)
		if errors.Is(err, orchestration.ErrConflict) {
			shared.Error(w, 409, "IDEMPOTENCY_CONFLICT")
			return
		}
		if err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		status := 200
		if created {
			status = 201
		}
		shared.JSON(w, status, stored)
	})
	for _, action := range []string{"enable", "disable", "run-now"} {
		mux.HandleFunc("POST /v1/schedules/{id}/"+action, func(w http.ResponseWriter, r *http.Request) {
			var input struct {
				Key string `json:"idempotency_key"`
			}
			if !shared.Decode(w, r, &input, 1024) {
				return
			}
			if !shared.ValidID(r.PathValue("id")) || !shared.ValidKey(input.Key) {
				shared.Error(w, 400, "INVALID_REQUEST")
				return
			}
			unlock := s.operations.Lock("schedule:" + r.PathValue("id"))
			defer unlock()
			schedule, err := repo.Schedule(r.Context(), r.PathValue("id"))
			if err != nil {
				shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
				return
			}
			if !s.AuthorizeProject(w, r, schedule.ProjectID) {
				return
			}
			if action == "run-now" {
				run, err := repo.Enqueue(r.Context(), schedule, "manual:"+input.Key, s.now())
				if err != nil {
					shared.Error(w, 503, "STORAGE_UNAVAILABLE")
					return
				}
				shared.JSON(w, 202, run)
				return
			}
			if action == "enable" && !schedule.Enabled {
				schedule.Enabled = true
				if schedule.NextRunAt.Before(s.now()) {
					schedule.NextRunAt = s.now()
				}
			}
			if action == "disable" {
				schedule.Enabled = false
			}
			if repo.SaveSchedule(r.Context(), schedule) != nil {
				shared.Error(w, 503, "STORAGE_UNAVAILABLE")
				return
			}
			shared.JSON(w, 200, schedule)
		})
	}
}

func (s *Server) tickOrchestration(ctx context.Context) {
	service := orchestration.Scheduler{Repository: orchestration.Repository{DB: s.Store.DB}, Workspaces: s.Store, Lifecycle: s.lifecycle(), Agent: s.Runtime, Now: s.now, Lock: s.operations.Lock, Cursor: &s.schedulerCursor}
	if err := service.Tick(ctx); err != nil {
		s.reportWrite("scheduling", "", err)
	}
}
