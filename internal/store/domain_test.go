package store

import (
	"context"
	"path/filepath"
	"testing"

	"agent-workbench/internal/shared"
)

func TestTaskMetadataCannotChangeWorkspace(t *testing.T) {
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "metadata.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	first := shared.Task{ID: "task_id", WorkspaceID: "ws_a", State: "running"}
	if err = s.SaveTask(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	forged := first
	forged.WorkspaceID = "ws_b"
	if err = s.SaveTask(context.Background(), forged); err != ErrOwnership {
		t.Fatal("task changed workspace")
	}
	items, err := s.Tasks(context.Background())
	if err != nil || len(items) != 1 || items[0].WorkspaceID != "ws_a" {
		t.Fatal("original task metadata changed")
	}
}
