package store

import (
	"agent-workbench/internal/shared"
	"context"
	"path/filepath"
	"testing"
)

func TestBoardUpgradePersistenceAndArchive(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "board.db")
	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`DROP TABLE board_items`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`DELETE FROM schema_migrations WHERE version=11`); err != nil {
		t.Fatal(err)
	}
	s.DB.Close()
	s, err = Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	item := BoardItem{ID: "work_a", ProjectID: "default", Stage: "archived", ArchivedFrom: "done", ExecutionState: "completed"}
	if err = s.SaveBoardItem(ctx, item, 0); err != nil {
		t.Fatal(err)
	}
	s.DB.Close()
	s, err = Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	item, err = s.BoardItem(ctx, "default", "work_a")
	if err != nil || item.Stage != "archived" || item.Revision != 1 {
		t.Fatal(item, err)
	}
	if item.Effective(shared.WorkItem{ID: item.ID, ProjectID: item.ProjectID, State: "running"}).Stage != "archived" {
		t.Fatal("worker activity unarchived card")
	}
	if item.RemoteState() != "closed" {
		t.Fatal("archive lost prior status")
	}
	if err = s.SaveBoardItem(ctx, item, 0); err != ErrBoardConflict {
		t.Fatal("revision conflict lost", err)
	}
}

func TestFinishedWorkOverridesPlanningPlacement(t *testing.T) {
	for _, state := range []string{"closed", "completed", "accepted"} {
		for _, stage := range []string{"planned", "running", "review", "done", "archived"} {
			t.Run(state+"/"+stage, func(t *testing.T) {
				// Reproduce a move saved after the work was already closed.
				b := BoardItem{Stage: stage, ExecutionState: state, ArchivedFrom: "running", Title: "Keep title", Revision: 4}
				work := shared.WorkItem{ID: "work_a", ProjectID: "default", State: state}
				got := b.Effective(work)
				if stage == "archived" {
					if got.Stage != "archived" || got.ArchivedFrom != "done" {
						t.Fatalf("archive lost finished state: %+v", got)
					}
				} else if got.Stage != "done" {
					t.Fatalf("finished task remains in %s", got.Stage)
				}
				if got.RemoteState() != "closed" || got.Title != b.Title || got.Revision != b.Revision {
					t.Fatalf("normalization changed metadata or remote outcome: %+v", got)
				}
				work.State = "running"
				if got.Effective(work).Stage != map[bool]string{true: "archived", false: "running"}[stage == "archived"] {
					t.Fatal("new execution did not restore active placement or preserve archive")
				}
			})
		}
	}
}
