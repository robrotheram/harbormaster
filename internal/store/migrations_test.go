package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestStartupMigrationsAreVersionedAndRepeatable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migration.db")
	for i := 0; i < 2; i++ {
		s, err := Open(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		var count int
		if err = s.DB.QueryRow(`SELECT count(*) FROM schema_migrations`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 7 {
			t.Fatalf("migration versions: %d", count)
		}
		for _, table := range []string{"work_items", "work_attempts", "schedules", "orchestration_runs", "workspaces", "agent_homes"} {
			if _, err = s.DB.Exec(`SELECT 1 FROM ` + table + ` LIMIT 1`); err != nil {
				t.Fatalf("schema unavailable before requests: %s: %v", table, err)
			}
		}
		if err = s.DB.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestToolProfileRetirementPreservesHistoricalRowsAndWorkspaceHome(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "tool-retirement.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	// Historical catalog rows stay inert after active profile support is removed.
	if _, err = s.DB.Exec(`INSERT INTO tool_profiles(id,revision,metadata) VALUES('old-custom',1,'{"mcps":[]}')`); err != nil {
		t.Fatal(err)
	}
	original := `{"id":"ws_old_custom","project_id":"default","home_id":"home_private_original","profile_id":"old-custom","agent_kind":"codex","state":"STOPPED"}`
	if _, err = s.DB.Exec(`INSERT INTO workspaces(id,create_key,metadata) VALUES('ws_old_custom','historical-create',$1)`, original); err != nil {
		t.Fatal(err)
	}
	if err = s.DB.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	var metadata string
	if err = s.DB.QueryRow(`SELECT metadata FROM workspaces WHERE id='ws_old_custom'`).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	if metadata != original {
		t.Fatal("retirement rewrote private workspace identity")
	}
	var rows int
	if err = s.DB.QueryRow(`SELECT count(*) FROM tool_profiles`).Scan(&rows); err != nil || rows != 1 {
		t.Fatal("historical profile rows changed or new seeds added", rows, err)
	}
	ws, err := s.Workspace(ctx, "ws_old_custom")
	if err != nil || ws.HomeID != "home_private_original" || ws.AgentKind != "codex" || ws.ConsoleEnabled {
		t.Fatal("legacy workspace reclassified", ws, err)
	}
}

func TestWorkMigrationUpgradesVersionTwoDatabase(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "upgrade.db")
	original, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"work_create_requests", "work_attempts", "work_items", "project_coordination", "exchange_exports", "project_exchanges"} {
		if _, err = original.DB.Exec(`DROP TABLE ` + table); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = original.DB.Exec(`DELETE FROM schema_migrations WHERE version>=3`); err != nil {
		t.Fatal(err)
	}
	if err = original.DB.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.DB.Close()
	if _, err = upgraded.DB.Exec(`SELECT metadata FROM work_items LIMIT 1`); err != nil {
		t.Fatal(err)
	}
	if _, err = upgraded.Project(ctx, "default"); err != nil {
		t.Fatal("existing project lost", err)
	}
}

func TestInlineWorkRequestMigrationUpgradesVersionSix(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "version-six.db")
	database, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	metadata := `{"id":"work_legacy","project_id":"default","workspace_id":"ws_legacy","state":"draft"}`
	if _, err = database.DB.Exec(`INSERT INTO work_items(id,project_id,create_key,metadata) VALUES('work_legacy','default','legacy',$1)`, metadata); err != nil {
		t.Fatal(err)
	}
	if _, err = database.DB.Exec(`DROP TABLE work_create_requests`); err != nil {
		t.Fatal(err)
	}
	if _, err = database.DB.Exec(`DELETE FROM schema_migrations WHERE version=7`); err != nil {
		t.Fatal(err)
	}
	if err = database.DB.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.DB.Close()
	if _, err = reopened.DB.Exec(`INSERT INTO work_create_requests(project_id,create_key,fingerprint) VALUES('default','request','fingerprint')`); err != nil {
		t.Fatal("request journal missing after upgrade", err)
	}
	var saved string
	if err = reopened.DB.QueryRow(`SELECT metadata FROM work_items WHERE id='work_legacy'`).Scan(&saved); err != nil || saved != metadata {
		t.Fatal("legacy work changed during upgrade", saved, err)
	}
}
