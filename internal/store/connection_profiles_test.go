package store

import (
	"agent-workbench/internal/shared"
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestEditedDefaultsSurviveDatabaseReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "profiles.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.ConnectionProfile(ctx, "github")
	if err != nil {
		t.Fatal(err)
	}
	p.Name = "Team GitHub"
	p.Revision++
	p.Policy = shared.ConnectionPolicy{Mode: "allowlist", Destinations: []string{"github.com:443", "example.com:443"}}
	if err = s.SaveConnectionProfile(ctx, p, 1); err != nil {
		t.Fatal(err)
	}
	s.DB.Close()
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	got, err := s.ConnectionProfile(ctx, "github")
	if err != nil || got.Name != p.Name || got.Revision != 2 || len(got.Policy.Destinations) != 2 {
		t.Fatal("seed defaults overwrote edited profile", err)
	}
}

func TestDeletedDefaultStaysDeletedAfterReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "profiles.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteConnectionProfile(ctx, "github", 1); err != nil {
		t.Fatal(err)
	}
	s.DB.Close()
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	if _, err := s.ConnectionProfile(ctx, "github"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("deleted default returned: %v", err)
	}
	profiles, err := s.ConnectionProfiles(ctx)
	if err != nil || len(profiles) != 3 {
		t.Fatalf("unexpected profiles after reopen: %v, %v", profiles, err)
	}
}
