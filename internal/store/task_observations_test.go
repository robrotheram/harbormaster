package store

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"agent-workbench/internal/shared"
)

func TestObservedTasksSkipUnchangedRows(t *testing.T) {
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "metadata.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	testObservedTasks(t, s)
	_, err = s.DB.ExecContext(t.Context(), `CREATE TRIGGER reject_task_update BEFORE UPDATE ON task_metadata
		BEGIN SELECT RAISE(ABORT, 'unexpected task rewrite'); END`)
	if err != nil {
		t.Fatal(err)
	}
	task := shared.Task{ID: "observation_task_001", WorkspaceID: "ws_observed", State: "running"}
	if err := s.SaveObservedTasks(t.Context(), task.WorkspaceID, []shared.Task{task}); err != nil {
		t.Fatalf("unchanged snapshot wrote to the database: %v", err)
	}
}

func testObservedTasks(t *testing.T, s *Store) {
	t.Helper()
	ctx := t.Context()
	foreign := shared.Task{ID: "observation_foreign", WorkspaceID: "ws_foreign", State: "queued"}
	if err := s.SaveTask(ctx, foreign); err != nil {
		t.Fatal(err)
	}
	tasks := make([]shared.Task, 251)
	for i := range tasks {
		tasks[i] = shared.Task{ID: fmt.Sprintf("observation_task_%03d", i), WorkspaceID: "ws_observed", State: "queued"}
	}
	if err := s.SaveObservedTasks(ctx, "ws_observed", tasks); err != nil {
		t.Fatal(err)
	}
	tasks[1].State = "running"
	conflict := foreign
	conflict.WorkspaceID = "ws_observed"
	if err := s.SaveObservedTasks(ctx, "ws_observed", append(tasks, conflict)); !errors.Is(err, ErrOwnership) {
		t.Fatalf("expected foreign ownership conflict, got %v", err)
	}
	all, err := s.Tasks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var owned int
	for _, task := range all {
		switch task.ID {
		case foreign.ID:
			if task != foreign {
				t.Fatal("foreign task changed")
			}
		case tasks[1].ID:
			if task.State != "running" {
				t.Fatal("valid change discarded with foreign task")
			}
		}
		if task.WorkspaceID == "ws_observed" {
			owned++
		}
	}
	if owned != len(tasks) {
		t.Fatalf("got %d observed tasks, want %d", owned, len(tasks))
	}
}
