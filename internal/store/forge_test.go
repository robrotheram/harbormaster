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
