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

func TestDefaultProjectProvisioningPreservesNormalProjectMetadata(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "metadata.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	items, err := s.Projects(ctx)
	if err != nil || len(items) != 1 || items[0].ID != "default" || items[0].Name != "Workspaces" {
		t.Fatal("unexpected projects on a fresh store", items, err)
	}
	project, err := s.RenameProject(ctx, "default", "My project")
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.DB.Close()
	persisted, err := reopened.Project(ctx, "default")
	if err != nil || persisted != project {
		t.Fatal("reopen replaced the default project", persisted, err)
	}
}
