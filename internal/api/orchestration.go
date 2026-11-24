package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/shared"
)

func (s *Server) registerOrchestration(mux *http.ServeMux) {
	repo := orchestration.Repository{DB: s.Store.DB}
	s.registerScheduleManagement(mux, repo)
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
		var input scheduleInput
		if !decodeRouteBody(w, r, &input) {
			return
		}
		if !s.AuthorizeProject(w, r, input.ProjectID) {
			return
		}
		if !input.valid() {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		unlock := s.operations.Lock("schedule-create:" + input.ProjectID)
		defer unlock()
		ws, err := s.Store.Workspace(r.Context(), input.WorkspaceID)
		if err != nil || !s.workspaceInProject(ws, input.ProjectID) {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		if input.Provider == "hermes" && ws.AgentKind != "hermes" {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		now := s.now()
		schedule := shared.Schedule{ID: shared.NewID("schedule_"), CreatorID: shared.WorkspaceQuotaUser(r.Context()), ProjectID: input.ProjectID, WorkspaceID: input.WorkspaceID, Name: input.Name, InstructionPath: input.InstructionPath, Provider: input.Provider, Enabled: true, RunAt: input.RunAt.UTC(), IntervalSeconds: input.Interval, NextRunAt: input.RunAt.UTC(), CreatedAt: now}
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
			if !decodeRouteBody(w, r, &input) {
				return
			}
			if !shared.ValidID(r.PathValue("id")) || !shared.ValidKey(input.Key) {
				shared.Error(w, 400, "INVALID_REQUEST")
				return
			}
			unlock := s.operations.Lock("schedule:" + r.PathValue("id"))
			defer unlock()
			schedule, err := repo.Schedule(r.Context(), r.PathValue("id"))
			if err != nil || schedule.DeletedAt != nil {
				shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
				return
			}
			if !s.AuthorizeProject(w, r, schedule.ProjectID) {
				return
			}
			if action != "disable" {
				ws, err := s.Store.Workspace(r.Context(), schedule.WorkspaceID)
				if err != nil || !s.workspaceInProject(ws, schedule.ProjectID) {
					shared.Error(w, 409, "SCHEDULE_WORKSPACE_UNAVAILABLE")
					return
				}
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
			wasEnabled := schedule.Enabled
			if action == "enable" && !schedule.Enabled {
				schedule.Enabled = true
				if schedule.NextRunAt.Before(s.now()) {
					schedule.NextRunAt = s.now()
				}
			}
			if action == "disable" {
				schedule.Enabled = false
			}
			if wasEnabled != schedule.Enabled {
				schedule.Revision++
			}
			if repo.SaveSchedule(r.Context(), schedule) != nil {
				shared.Error(w, 503, "STORAGE_UNAVAILABLE")
				return
			}
			shared.JSON(w, 200, schedule)
		})
	}
}

func (s *Server) scheduler(repo orchestration.Repository) orchestration.Scheduler {
	return orchestration.Scheduler{Repository: repo, Workspaces: s.Store, Lifecycle: s.lifecycle(), Agent: s.Runtime, Now: s.now, Lock: s.operations.Lock, Cursor: &s.schedulerCursor, CanUseWorkspace: s.workspaceInProject, ProjectAvailable: func(ctx context.Context, project string) (bool, error) {
		_, err := s.Store.Project(ctx, project)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return err == nil, err
	}}
}

func (s *Server) tickOrchestration(ctx context.Context) {
	service := s.scheduler(orchestration.Repository{DB: s.Store.DB})
	if err := service.Tick(ctx); err != nil {
		s.reportWrite("scheduling", "", err)
	}
}

// The database holds the queue. A pending signal covers all submissions made
// before the next scan, and periodic ticks recover missed signals and retries.
func (s *Server) notifyWorkReady() {
	select {
	case s.workReady <- struct{}{}:
	default:
	}
}

func (s *Server) runOrchestration(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	repo := orchestration.Repository{DB: s.Store.DB}
	service := s.scheduler(repo)
	tick := func() {
		tickCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		s.tickOrchestration(tickCtx)
	}
	tick()
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tick()
		case <-s.workReady:
			dispatchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			s.reportWrite("work_dispatch", "", repo.TickWork(dispatchCtx, &service))
			cancel()
		}
	}
}
