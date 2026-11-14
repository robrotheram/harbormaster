package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestForgeConnectionsPersistAtPlatformScope(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "store.db")
	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	c := ForgeConnection{ID: "forge_one", Host: "github", Repository: "team/repo"}
	if err = s.SaveForge(ctx, c); err != nil {
		t.Fatal(err)
	}
	s.DB.Close()
	s, err = Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	items, err := s.ForgeConnections(ctx)
	if err != nil || len(items) != 1 || items[0] != c {
		t.Fatal(items, err)
	}
	if err = s.DeleteForge(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	items, _ = s.ForgeConnections(ctx)
	if len(items) != 0 {
		t.Fatal(items)
	}
}

func TestForgeMigrationUpgradesExistingDatabase(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "legacy.db")
	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	// Reproduce a database where the original eight migrations already ran.
	if _, err = s.DB.ExecContext(ctx, `DROP TABLE forge_connections`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version=9`); err != nil {
		t.Fatal(err)
	}
	s.DB.Close()
	s, err = Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	if _, err = s.ForgeConnections(ctx); err != nil {
		t.Fatal("upgrade did not create repository connections", err)
	}
	var count int
	if err = s.DB.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations WHERE version=9`).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
}

func TestLegacyProjectConnectionsRemainAvailable(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	_, err = s.DB.ExecContext(ctx, `INSERT INTO forge_connections(id,project_id,metadata) VALUES('forge_legacy','old_project','{"id":"forge_legacy","project_id":"old_project","host":"office","repository":"team/repo"}')`)
	if err != nil {
		t.Fatal(err)
	}
	items, err := s.ForgeConnections(ctx)
	if err != nil || len(items) != 1 || items[0].ID != "forge_legacy" {
		t.Fatal(items, err)
	}
}
