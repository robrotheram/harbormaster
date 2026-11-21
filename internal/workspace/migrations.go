package workspace

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"agent-workbench/internal/workspace/taskstore"
)

// ErrPreReleaseSchema identifies a database that needs recreation before startup.
var ErrPreReleaseSchema = errors.New("incompatible pre-release workspace database: use a fresh workspace database; see docs/operations.md")

// The supervisor holds the workspace writer lock throughout database startup.
func initWorkspaceDB(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var legacy bool
	if err := tx.QueryRowContext(ctx, `SELECT
		NOT EXISTS (SELECT 1 FROM sqlite_schema WHERE type='table' AND name='schema_migrations')
		AND EXISTS (SELECT 1 FROM sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%')`).Scan(&legacy); err != nil {
		return err
	}
	if legacy {
		return ErrPreReleaseSchema
	}
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY CHECK(version > 0))`); err != nil {
		return err
	}
	var version int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version); err != nil {
		return err
	}

	steps := []func(context.Context, *sql.Tx) error{initWorkspaceSchema, addReasoningEffort, addTaskChanges}
	if version > len(steps) {
		return fmt.Errorf("workspace schema version %d is newer than this binary supports (%d)", version, len(steps))
	}
	for i, apply := range steps[version:] {
		next := version + i + 1
		if err := apply(ctx, tx); err != nil {
			return fmt.Errorf("workspace migration %d: %w", next, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version) VALUES(?)`, next); err != nil {
			return err
		}
	}

	if err := taskstore.RecoverInterrupted(ctx, tx); err != nil {
		return err
	}
	for _, q := range []string{
		`UPDATE retained_services SET state='stopped' WHERE id IN (SELECT service_id FROM workspace_applications) AND state='active'`,
	} {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func initWorkspaceSchema(ctx context.Context, tx *sql.Tx) error {
	for _, q := range []string{
		taskstore.Schema,
		`CREATE TABLE agent_history(seq INTEGER PRIMARY KEY AUTOINCREMENT,id TEXT UNIQUE NOT NULL,episode_id TEXT NOT NULL,body TEXT NOT NULL)`,
		`CREATE INDEX agent_history_episode ON agent_history(episode_id)`,
		`CREATE TABLE history_state(id INTEGER PRIMARY KEY,bytes_used INTEGER NOT NULL DEFAULT 0,limited INTEGER NOT NULL DEFAULT 0)`,
		`INSERT INTO history_state(id) VALUES(1)`,
		`CREATE TABLE activity_events(id TEXT PRIMARY KEY,at TEXT NOT NULL,body TEXT NOT NULL)`,
		`CREATE TABLE retained_service_receipts(idem_key TEXT PRIMARY KEY,fingerprint TEXT NOT NULL,service_id TEXT NOT NULL,source_ref TEXT NOT NULL,ttl INTEGER NOT NULL)`,
		`CREATE TABLE retained_services(id TEXT PRIMARY KEY,state TEXT NOT NULL,expires_at INTEGER NOT NULL,snapshot BLOB NOT NULL)`,
		`CREATE TABLE workspace_applications(service_id TEXT PRIMARY KEY,idem_key TEXT UNIQUE NOT NULL,fingerprint TEXT NOT NULL,configuration TEXT NOT NULL)`,
		`CREATE TABLE artifacts(id TEXT PRIMARY KEY,idem_key TEXT UNIQUE NOT NULL,source_path TEXT NOT NULL,sha256 TEXT NOT NULL,size_bytes INTEGER NOT NULL,created_at TEXT NOT NULL,content BLOB NOT NULL)`,
	} {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

func addReasoningEffort(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, taskstore.ReasoningSchema)
	return err
}

func addTaskChanges(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, taskstore.ChangesSchema)
	return err
}
