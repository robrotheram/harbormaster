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
		if count != 17 {
			t.Fatalf("migration versions: %d", count)
		}
		for _, table := range []string{"project_members", "agent_subscriptions", "worker_agent_subscriptions", "work_review_history", "forge_accounts", "work_items", "work_attempts", "schedules", "orchestration_runs", "workspaces", "agent_homes"} {
			if _, err = s.DB.Exec(`SELECT 1 FROM ` + table + ` LIMIT 1`); err != nil {
				t.Fatalf("schema unavailable before requests: %s: %v", table, err)
			}
		}
		if err = s.DB.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWorkspaceConnectionMigrationPreservesEffectivePolicy(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "workspace-connections.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	checkWorkspaceConnectionMigration(t, ctx, s)
}

func checkWorkspaceConnectionMigration(t *testing.T, ctx context.Context, s *Store) {
	t.Helper()
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version=17`); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		id, metadata, mode string
	}{
		{"legacy-enabled", `{"github_access":true}`, "allowlist"},
		{"legacy-disabled", `{"github_access":false}`, "blocked"},
		{"explicit-blocked", `{"github_access":true,"connections":{"mode":"blocked","destinations":[]}}`, "blocked"},
	}
	for _, tt := range tests {
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO workspaces(id,create_key,metadata) VALUES($1,$1,$2)`, tt.id, tt.metadata); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.migrate(ctx); err != nil {
		t.Fatal(err)
	}
	for _, tt := range tests {
		w, err := s.Workspace(ctx, tt.id)
		if err != nil {
			t.Fatal(err)
		}
		p := w.EffectiveConnections()
		if p.Mode != tt.mode || p.Allows("github.com:443") != (tt.mode == "allowlist") || p.Allows("registry.npmjs.org:443") {
			t.Errorf("%s: unexpected policy %+v", tt.id, p)
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

func TestSubscriptionMigrationRetainsDefaultLoginAndBinding(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "subscriptions.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.initSubscriptions(ctx); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO user_subscriptions(owner,credential) VALUES('user_old','original-encrypted-login')`,
		`INSERT INTO worker_subscriptions(workspace_id,owner) VALUES('ws_old','user_old')`,
		`DROP TABLE worker_agent_subscriptions`, `DROP TABLE agent_subscriptions`,
		`DELETE FROM schema_migrations WHERE version=14`,
	} {
		if _, err = s.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	s.DB.Close()
	for i := 0; i < 2; i++ {
		s, err = Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		v, err := s.AgentSubscription(ctx, "user_old", "default")
		if err != nil || v.Credential != "original-encrypted-login" || v.Provider != "codex" {
			t.Fatal("lost migrated login", v, err)
		}
		owner, id, err := s.WorkerAgentSubscription(ctx, "ws_old", "codex")
		if err != nil || owner != "user_old" || id != "default" {
			t.Fatal("lost migrated binding", err)
		}
		s.DB.Close()
	}
}
