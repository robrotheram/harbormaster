package orchestration

import (
	"agent-workbench/internal/shared"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"
)

type ScheduleRepository interface {
	Schedules(context.Context, string) ([]shared.Schedule, error)
	Schedule(context.Context, string) (shared.Schedule, error)
	DeliverDue(context.Context, string, time.Time) error
	SaveSchedule(context.Context, shared.Schedule) error
	PendingRunsAfter(context.Context, string, int) ([]shared.Run, error)
	SaveRun(context.Context, shared.Run) error
}
type WorkspaceRepository interface {
	Workspace(context.Context, string) (shared.Workspace, error)
	SaveTask(context.Context, shared.Task) error
}
type WorkspaceLifecycle interface {
	Refresh(context.Context, *shared.Workspace) error
	Start(context.Context, *shared.Workspace) error
}
type Agent interface {
	Agent(context.Context, string, string, string, []byte) (int, []byte, error)
}

// Cursor advances over every pending run, including unavailable workspaces. The
// next tick wraps, so old active work cannot permanently hide newer occurrences.
type Cursor struct {
	mu    sync.Mutex
	after string
}
type Scheduler struct {
	Repository ScheduleRepository
	Workspaces WorkspaceRepository
	Lifecycle  WorkspaceLifecycle
	Agent      Agent
	Now        func() time.Time
	Lock       func(...string) func()
	Cursor     *Cursor
}

func (s *Scheduler) Tick(ctx context.Context) error {
	s.Cursor.mu.Lock()
	defer s.Cursor.mu.Unlock()
	var failures error
	if work, ok := s.Repository.(interface {
		TickWork(context.Context, *Scheduler) error
	}); ok {
		failures = work.TickWork(ctx, s)
	}
	report := func(stage, id string, err error) {
		if err != nil {
			slog.Error("scheduler persistence failed", "stage", stage, "resource_id", id, "code", "STORAGE_UNAVAILABLE")
			failures = errors.Join(failures, errors.New("scheduling persistence failed"))
		}
	}
	repo := s.Repository
	schedules, err := repo.Schedules(ctx, "")
	if err != nil {
		return err
	}
	now := s.Now()
	for _, schedule := range schedules {
		if err := ctx.Err(); err != nil {
			return errors.Join(failures, err)
		}
		func() {
			unlock := s.Lock("schedule:" + schedule.ID)
			defer unlock()
			report("due_occurrence", schedule.ID, repo.DeliverDue(ctx, schedule.ID, now))
		}()
	}

	runs, err := repo.PendingRunsAfter(ctx, s.Cursor.after, 100)
	if err != nil {
		return err
	}
	if len(runs) == 0 {
		s.Cursor.after = ""
	}
	for _, run := range runs {
		if err := ctx.Err(); err != nil {
			return errors.Join(failures, err)
		}
		s.Cursor.after = run.ID
		func() {
			unlock := s.Lock("schedule:"+run.ScheduleID, run.WorkspaceID)
			defer unlock()
			if !shared.Active(run.State) && run.State != "dispatching" {
				return
			}
			schedule, err := repo.Schedule(ctx, run.ScheduleID)
			if err != nil {
				report("schedule_read", run.ScheduleID, err)
				return
			}
			// A deletion can cancel queued runs after this tick loaded its batch.
			if schedule.DeletedAt != nil && run.State == "queued" && run.TaskID == "" {
				return
			}
			ws, err := s.Workspaces.Workspace(ctx, run.WorkspaceID)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				// Storage outages say nothing about ownership or execution. Keep
				// accepted work pending and retry its existing identity next time.
				report("workspace_read", run.WorkspaceID, err)
				return
			}
			if err != nil || ws.ProjectID != run.ProjectID || schedule.ProjectID != run.ProjectID {
				run.State = "failed"
				run.ErrorCode = "NOT_FOUND_OR_FORBIDDEN"
				run.UpdatedAt = now
				report("run", run.ID, repo.SaveRun(ctx, run))
				return
			}
			if run.TaskID != "" {
				status, body, err := s.Agent.Agent(ctx, ws.ID, "GET", "/v1/tasks/"+run.TaskID, nil)
				var task shared.Task
				if err == nil && status == 200 && json.Unmarshal(body, &task) == nil && shared.ValidTask(task, ws.ID) && task.ID == run.TaskID {
					run.State = task.State
					run.ErrorCode = shared.SanitizedTaskError(task.ErrorCode)
					run.UpdatedAt = now
					report("run", run.ID, repo.SaveRun(ctx, run))
				}
				return
			}
			if err = s.Lifecycle.Refresh(ctx, &ws); err != nil {
				return
			}
			if ws.State == "BUSY" || ws.State == "UNKNOWN" {
				return
			}
			if err = s.Lifecycle.Start(ctx, &ws); err != nil {
				run.ErrorCode = "WORKSPACE_UNAVAILABLE"
				if err.Error() == "QUOTA_EXCEEDED" {
					run.ErrorCode = "QUOTA_EXCEEDED"
				}
				run.UpdatedAt = now
				report("run", run.ID, repo.SaveRun(ctx, run))
				return
			}
			run.State = "dispatching"
			run.UpdatedAt = now
			if err := repo.SaveRun(ctx, run); err != nil {
				report("dispatch", run.ID, err)
				return
			}
			body, _ := json.Marshal(map[string]string{"instruction_path": schedule.InstructionPath, "provider": schedule.Provider, "idempotency_key": "schedule:" + run.ID})
			status, reply, err := s.Agent.Agent(ctx, ws.ID, "POST", "/v1/tasks/from-file", body)
			var task shared.Task
			if err != nil {
				return
			} // Lost acknowledgements retry the same durable workspace key.
			if (status == 200 || status == 202) && json.Unmarshal(reply, &task) == nil && shared.ValidTask(task, ws.ID) {
				run.TaskID = task.ID
				run.State = task.State
				run.ErrorCode = ""
				report("task", task.ID, s.Workspaces.SaveTask(ctx, task))
				schedule.LastRunAt = &now
				schedule.LastErrorCode = ""
			} else if status == 409 {
				run.State = "unknown"
				run.ErrorCode = "ACCEPTANCE_CONFLICT"
				schedule.LastErrorCode = run.ErrorCode
			} else if status >= 400 && status < 500 {
				run.State = "failed"
				run.ErrorCode = "INSTRUCTION_FILE_UNAVAILABLE"
				schedule.LastErrorCode = run.ErrorCode
			} else {
				run.State = "dispatching"
				run.ErrorCode = "WORKSPACE_UNAVAILABLE"
			}
			run.UpdatedAt = now
			report("run", run.ID, repo.SaveRun(ctx, run))
			report("schedule", schedule.ID, repo.SaveSchedule(ctx, schedule))

		}()
	}
	if len(runs) < 100 {
		s.Cursor.after = ""
	}
	return failures
}
