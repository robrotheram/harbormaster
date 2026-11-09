package orchestration

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"agent-workbench/internal/shared"
)

type recoveringWorkspaces struct {
	err       error
	workspace shared.Workspace
	observed  shared.Task
}

func (s *recoveringWorkspaces) Workspace(context.Context, string) (shared.Workspace, error) {
	return s.workspace, s.err
}
func (s *recoveringWorkspaces) SaveTask(_ context.Context, task shared.Task) error {
	s.observed = task
	return nil
}

type finishedAgent struct{ task shared.Task }

func (a finishedAgent) Agent(context.Context, string, string, string, []byte) (int, []byte, error) {
	body, err := json.Marshal(a.task)
	return 200, body, err
}

func TestTransientWorkspaceReadDoesNotAbandonAcceptedRun(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "recovery.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := Repository{DB: db}
	if err := repo.Init(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	schedule := shared.Schedule{ID: "schedule_recovery", ProjectID: "project_recovery", WorkspaceID: "ws_recovery", CreatedAt: now}
	if _, _, err := repo.CreateSchedule(ctx, schedule, "recovery"); err != nil {
		t.Fatal(err)
	}
	run, err := repo.Enqueue(ctx, schedule, "accepted", now)
	if err != nil {
		t.Fatal(err)
	}
	run.TaskID, run.State = "task_accepted", "running"
	if err := repo.SaveRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	workspaces := &recoveringWorkspaces{err: context.DeadlineExceeded, workspace: shared.Workspace{ID: run.WorkspaceID, ProjectID: run.ProjectID}}
	task := shared.Task{ID: run.TaskID, WorkspaceID: run.WorkspaceID, State: "succeeded"}
	scheduler := Scheduler{Repository: repo, Workspaces: workspaces, Agent: finishedAgent{task: task}, Now: func() time.Time { return now }, Lock: func(...string) func() { return func() {} }, Cursor: &Cursor{}}
	if err := scheduler.Tick(ctx); err == nil {
		t.Fatal("storage outage was hidden")
	}
	runs, err := repo.Runs(ctx, run.ProjectID)
	if err != nil || len(runs) != 1 || runs[0].State != "running" || runs[0].TaskID != run.TaskID {
		t.Fatal("transient read abandoned accepted execution", runs, err)
	}
	workspaces.err = nil
	// The pending cursor wraps; neither recovery nor observation may resubmit.
	for i := 0; i < 2; i++ {
		if err := scheduler.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	runs, err = repo.Runs(ctx, run.ProjectID)
	if err != nil || len(runs) != 1 || runs[0].ID != run.ID || runs[0].State != "succeeded" || runs[0].TaskID != run.TaskID {
		t.Fatal("execution did not recover with its accepted identity", runs, err)
	}
	// A genuinely missing owner remains a terminal failure, rather than a retry loop.
	run.State = "running"
	if err := repo.SaveRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	workspaces.err = errors.Join(errors.New("lookup failed"), sql.ErrNoRows)
	for i := 0; i < 2; i++ {
		if err := scheduler.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	runs, err = repo.Runs(ctx, run.ProjectID)
	if err != nil || len(runs) != 1 || runs[0].State != "failed" || runs[0].ErrorCode != "NOT_FOUND_OR_FORBIDDEN" {
		t.Fatal("missing workspace was not classified separately", runs, err)
	}
}
