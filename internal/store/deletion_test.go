package store

import (
	"agent-workbench/internal/shared"
	"context"
	"path/filepath"
	"testing"
)

func TestDeletedProjectRejectsLateWorkspaceCreation(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "metadata.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	if err := s.DeleteStopped(ctx, nil, "default"); err != nil {
		t.Fatal(err)
	}
	ws := shared.Workspace{ID: "late_workspace", ProjectID: "default", HomeID: "late_home", AgentKind: "t3", State: "STOPPED"}
	if err := s.Create(ctx, ws, "late-create"); err == nil {
		t.Fatal("workspace created in deleted project")
	}
}
