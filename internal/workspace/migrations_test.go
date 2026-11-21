package workspace

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceStartupUsesCurrentSchema(t *testing.T) {
	cfg := Config{ID: "ws_test", HomeID: "home_test", Token: "test-workspace-token", DataDir: t.TempDir()}
	for i := 0; i < 2; i++ {
		m, err := New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		var count, version int
		err = m.db.QueryRow(`SELECT count(*), max(version) FROM schema_migrations`).Scan(&count, &version)
		m.Close()
		if err != nil || count != 3 || version != 3 {
			t.Fatal("expected workspace schema version 3", count, version, err)
		}
	}
}

func TestWorkspaceStartupRejectsPreReleaseDatabase(t *testing.T) {
	cfg := Config{ID: "ws_test", HomeID: "home_test", Token: "test-workspace-token", DataDir: t.TempDir()}
	if err := os.MkdirAll(homePath(cfg), 0700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(homePath(cfg), "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE factory_schema_migrations(version INTEGER PRIMARY KEY);
		INSERT INTO factory_schema_migrations VALUES(1)`); err != nil {
		t.Fatal(err)
	}
	if m, err := New(cfg); err == nil {
		m.Close()
		t.Fatal("opened pre-release workspace")
	} else if !errors.Is(err, ErrPreReleaseSchema) {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE type='table'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("startup changed the pre-release schema", count, err)
	}
}
