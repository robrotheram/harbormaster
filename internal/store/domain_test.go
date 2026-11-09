package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"agent-workbench/internal/shared"
)

func TestLegacyReopenPreservesRuntimePathsAndPrivateIdentity(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "metadata.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	old := shared.Workspace{ID: "ws_existing", ProjectID: "default", Name: "Do not classify by this name", ProfileID: "gallery", State: "STOPPED", CreatedAt: time.Now().UTC()}
	b, _ := json.Marshal(old)
	if _, err = s.DB.Exec(`INSERT INTO workspaces(id,create_key,metadata) VALUES(?,?,?)`, old.ID, "old-key", string(b)); err != nil {
		t.Fatal(err)
	}
	s.DB.Close()
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	ws, err := s.Workspace(ctx, old.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ws.ProjectID != "default" || ws.HomeID != "" || ws.Name != old.Name || ws.AgentKind != "t3" {
		t.Fatal("legacy workspace silently migrated")
	}
	homes, err := s.AgentHomes(ctx, "default")
	if err != nil || len(homes) != 1 || homes[0].WorkspaceID != old.ID || homes[0].ProjectID != "default" {
		t.Fatal("legacy home metadata missing")
	}
	p, err := s.Project(ctx, "default")
	if err != nil || !p.Legacy {
		t.Fatal("legacy project not labeled")
	}
	if err = s.Save(ctx, ws); err != nil {
		t.Fatal(err)
	}
	if ws.HomeID != "" {
		t.Fatal("legacy storage path changed")
	}
}
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
