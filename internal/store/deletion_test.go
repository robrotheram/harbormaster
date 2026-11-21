package store

import (
	"agent-workbench/internal/shared"
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestDeletedProjectRejectsLateWorkspaceCreation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "metadata.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	if err := s.DeleteStopped(ctx, nil, "default"); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.DB.Close()
	if _, err := reopened.Project(ctx, "default"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("reopen should keep default project deleted: %v", err)
	}
	ws := shared.Workspace{ID: "late_workspace", ProjectID: "default", HomeID: "late_home", AgentKind: "t3", State: "STOPPED"}
	if err := reopened.Create(ctx, ws, "late-create"); err == nil {
		t.Fatal("workspace created in deleted project")
	}
}
