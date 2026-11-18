package store

import (
	"context"
	"database/sql"
	"errors"
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
		var count, version int
		if err = s.DB.QueryRow(`SELECT count(*), max(version) FROM schema_migrations`).Scan(&count, &version); err != nil {
			t.Fatal(err)
		}
		if count != 1 || version != 1 {
			t.Fatalf("migration versions: count %d, latest %d", count, version)
		}
		for _, table := range []string{"project_members", "agent_subscriptions", "worker_agent_subscriptions", "work_review_history", "forge_accounts", "work_items", "work_attempts", "schedules", "orchestration_runs", "workspaces"} {
			if _, err = s.DB.Exec(`SELECT 1 FROM ` + table + ` LIMIT 1`); err != nil {
				t.Fatalf("schema unavailable before requests: %s: %v", table, err)
			}
		}
		if err = s.DB.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPreReleaseMetadataRequiresFreshDatabase(t *testing.T) {
	for name, statements := range map[string][]string{
		"old version one": {
			`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY)`,
			`INSERT INTO schema_migrations VALUES(1)`,
		},
		"unversioned": {
			`CREATE TABLE workspaces(id TEXT PRIMARY KEY)`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "legacy.db")
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			for _, q := range statements {
				if _, err := db.Exec(q); err != nil {
					t.Fatal(err)
				}
			}
			if s, err := Open(ctx, path); err == nil {
				s.DB.Close()
				t.Fatal("opened pre-release metadata")
			} else if !errors.Is(err, ErrPreReleaseSchema) {
				t.Fatal(err)
			}
			var count int
			if err := db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE type='table'`).Scan(&count); err != nil || count != 1 {
				t.Fatal("startup changed the pre-release schema", count, err)
			}
		})
	}
}
