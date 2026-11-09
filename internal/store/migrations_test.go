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
		if count != 2 {
			t.Fatalf("migration versions: %d", count)
		}
		for _, table := range []string{"schedules", "orchestration_runs", "workspaces", "agent_homes"} {
			if _, err = s.DB.Exec(`SELECT 1 FROM ` + table + ` LIMIT 1`); err != nil {
				t.Fatalf("schema unavailable before requests: %s: %v", table, err)
			}
		}
		if err = s.DB.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
