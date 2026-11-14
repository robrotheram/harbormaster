package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestForgeAccountsUpgradeExistingDatabase(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "upgrade.db")
	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveForge(ctx, ForgeConnection{ID: "forge_existing", Host: "office", Repository: "team/repo"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`DROP TABLE forge_accounts`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`DELETE FROM schema_migrations WHERE version=10`); err != nil {
		t.Fatal(err)
	}
	s.DB.Close()
	s, err = Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	if _, err = s.ForgeAccounts(ctx); err != nil {
		t.Fatal(err)
	}
	connections, err := s.ForgeConnections(ctx)
	if err != nil || len(connections) != 1 || connections[0].ID != "forge_existing" {
		t.Fatal("upgrade lost existing connections", err)
	}
}
