package api

import (
	"net/http"
	"strings"
	"time"

	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/shared"
)

type scheduleInput struct {
	ProjectID       string    `json:"project_id"`
	WorkspaceID     string    `json:"workspace_id"`
	Name            string    `json:"name"`
	InstructionPath string    `json:"instruction_path"`
	Provider        string    `json:"provider"`
	RunAt           time.Time `json:"run_at"`
	Interval        int64     `json:"interval_seconds"`
	Revision        int       `json:"revision"`
	Key             string    `json:"idempotency_key"`
}

func (i *scheduleInput) valid() bool {
	i.Name = strings.TrimSpace(i.Name)
	if i.Provider == "" {
		i.Provider = "codex"
	}
	return shared.ValidID(i.WorkspaceID) && shared.ValidKey(i.Key) && i.Name != "" && len(i.Name) <= 80 && shared.ValidProjectFile(i.InstructionPath) && (i.Provider == "codex" || i.Provider == "hermes") && !i.RunAt.IsZero() && i.Interval >= 0 && (i.Interval == 0 || i.Interval >= 60) && i.Interval <= 31536000 && i.Revision >= 0
}

func (s *Server) registerScheduleManagement(mux *http.ServeMux, repo orchestration.Repository) {
	mux.HandleFunc("POST /v1/schedules/{id}/update", func(w http.ResponseWriter, r *http.Request) {
		var input scheduleInput
		if !shared.Decode(w, r, &input, 4096) {
			return
		}
		if !shared.ValidID(r.PathValue("id")) || !input.valid() {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		unlock := s.operations.Lock("schedule:" + r.PathValue("id"))
		defer unlock()
		current, err := repo.Schedule(r.Context(), r.PathValue("id"))
		if err != nil || current.DeletedAt != nil {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		if !s.AuthorizeProject(w, r, current.ProjectID) {
			return
		}
		if input.ProjectID != current.ProjectID {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		ws, err := s.Store.Workspace(r.Context(), input.WorkspaceID)
		if err != nil || !s.workspaceInProject(ws, current.ProjectID) {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		if input.Provider == "hermes" && ws.AgentKind != "hermes" {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		// Identical retries must not rewind a trigger already advanced by the scheduler.
		unchanged := current.Name == input.Name && current.WorkspaceID == input.WorkspaceID && current.InstructionPath == input.InstructionPath && current.Provider == input.Provider && current.RunAt.Equal(input.RunAt) && current.IntervalSeconds == input.Interval
		if unchanged {
			shared.JSON(w, 200, current)
			return
		}
		if current.Revision != input.Revision {
			shared.Error(w, 409, "SCHEDULE_REVISION_CONFLICT")
			return
		}
		active, err := repo.HasUnfinishedRuns(r.Context(), current.ID)
		if err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		if active {
			shared.Error(w, 409, "SCHEDULE_HAS_UNFINISHED_RUNS")
			return
		}
		if !current.RunAt.Equal(input.RunAt) || current.IntervalSeconds != input.Interval {
			current.NextRunAt = input.RunAt.UTC()
		}
		current.Name, current.WorkspaceID = input.Name, input.WorkspaceID
		current.InstructionPath, current.Provider = input.InstructionPath, input.Provider
		current.RunAt, current.IntervalSeconds = input.RunAt.UTC(), input.Interval
		current.Revision++
		if repo.SaveSchedule(r.Context(), current) != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, current)
	})
	mux.HandleFunc("POST /v1/schedules/{id}/delete", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Revision int    `json:"revision"`
			Key      string `json:"idempotency_key"`
		}
		if !shared.Decode(w, r, &input, 1024) {
			return
		}
		if !shared.ValidID(r.PathValue("id")) || !shared.ValidKey(input.Key) || input.Revision < 0 {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		unlock := s.operations.Lock("schedule:" + r.PathValue("id"))
		defer unlock()
		current, err := repo.Schedule(r.Context(), r.PathValue("id"))
		if err != nil {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		if !s.AuthorizeProject(w, r, current.ProjectID) {
			return
		}
		if current.DeletedAt == nil {
			if current.Revision != input.Revision {
				shared.Error(w, 409, "SCHEDULE_REVISION_CONFLICT")
				return
			}
			if repo.DeleteSchedule(r.Context(), current, s.now()) != nil {
				shared.Error(w, 503, "STORAGE_UNAVAILABLE")
				return
			}
		}
		shared.JSON(w, 200, map[string]bool{"deleted": true})
	})
}
