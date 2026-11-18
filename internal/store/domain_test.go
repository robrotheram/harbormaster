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
	if _, err = s.DB.Exec(`INSERT INTO workspaces(id,create_key,metadata) VALUES($1,$2,$3)`, old.ID, "old-key", string(b)); err != nil {
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

func TestRetiredHomeCatalogDoesNotChangeWorkerStorage(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "retired.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	original := `{"id":"home_old","workspace_id":"ws_old","project_id":"default"}`
	if _, err := s.DB.Exec(`INSERT INTO agent_homes(id,project_id,workspace_id,metadata) VALUES('home_old','default','ws_old',$1)`, original); err != nil {
		t.Fatal(err)
	}
	ws := shared.Workspace{ID: "ws_new", ProjectID: "default", HomeID: "home_new", AgentKind: "codex", State: "STOPPED", CreatedAt: time.Now()}
	if err := s.Create(ctx, ws, "new-worker"); err != nil {
		t.Fatal(err)
	}
	s.DB.Close()
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	stored, err := s.Workspace(ctx, ws.ID)
	if err != nil || stored.HomeID != ws.HomeID {
		t.Fatal("worker storage identity changed", stored, err)
	}
	var count int
	if err := s.DB.QueryRow(`SELECT count(*) FROM agent_homes`).Scan(&count); err != nil || count != 1 {
		t.Fatal("catalog still being populated", count, err)
	}
	var historical string
	if err := s.DB.QueryRow(`SELECT metadata FROM agent_homes WHERE id='home_old'`).Scan(&historical); err != nil || historical != original {
		t.Fatal("historical metadata changed", err)
	}
}
