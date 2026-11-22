package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"agent-workbench/internal/coordination"
	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/shared"
	"github.com/jackc/pgx/v5/stdlib"
)

// ErrPreReleaseSchema identifies a database that needs recreation before startup.
var ErrPreReleaseSchema = errors.New("incompatible pre-release metadata database: configure a fresh database; see docs/operations.md")

// Migrations run before listeners or workers start.
func (s *Store) migrate(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, postgres := s.DB.Driver().(*stdlib.Driver)
	if postgres {
		// Serialize initial schema creation when two processes open the database.
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(1489778321)`); err != nil {
			return err
		}
	}
	const ledger = `CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY CHECK(version > 0), applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)`
	if _, err := tx.ExecContext(ctx, ledger); err != nil {
		return err
	}

	// The old ledger also used version 1. Its missing timestamp distinguishes
	// every historical version from the new migration sequence.
	checkLedger := `SELECT EXISTS (SELECT 1 FROM pragma_table_info('schema_migrations') WHERE name='applied_at')`
	checkLegacyTables := `SELECT EXISTS (SELECT 1 FROM sqlite_schema WHERE type='table' AND name IN ('workspaces','deleted_resources'))`
	if postgres {
		checkLedger = `SELECT EXISTS (SELECT 1 FROM pg_attribute WHERE attrelid='schema_migrations'::regclass AND attname='applied_at' AND NOT attisdropped)`
		checkLegacyTables = `SELECT to_regclass('workspaces') IS NOT NULL OR to_regclass('deleted_resources') IS NOT NULL`
	}
	var currentLedger bool
	if err := tx.QueryRowContext(ctx, checkLedger).Scan(&currentLedger); err != nil {
		return err
	}
	if !currentLedger {
		return ErrPreReleaseSchema
	}

	var version int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version); err != nil {
		return err
	}
	if version == 0 {
		var legacyTables bool
		if err := tx.QueryRowContext(ctx, checkLegacyTables).Scan(&legacyTables); err != nil {
			return err
		}
		if legacyTables {
			return ErrPreReleaseSchema
		}
	}

	steps := []func(context.Context, *sql.Tx) error{s.initMetadata, initSubscriptionDefaults, s.migrateHistoryCache}
	if version > len(steps) {
		return fmt.Errorf("metadata schema version %d is newer than this binary supports (%d)", version, len(steps))
	}
	for i, apply := range steps[version:] {
		next := version + i + 1
		if err := apply(ctx, tx); err != nil {
			return fmt.Errorf("metadata migration %d: %w", next, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version) VALUES($1)`, next); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) initMetadata(ctx context.Context, tx *sql.Tx) error {
	for _, q := range []string{
		`CREATE TABLE deleted_resources(kind TEXT NOT NULL,id TEXT NOT NULL,PRIMARY KEY(kind,id))`,
		`CREATE TABLE workspaces (id TEXT PRIMARY KEY, create_key TEXT UNIQUE NOT NULL, metadata TEXT NOT NULL)`,
		`CREATE TABLE task_metadata (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, metadata TEXT NOT NULL)`,
		`CREATE TABLE projects(id TEXT PRIMARY KEY,create_key TEXT UNIQUE NOT NULL,metadata TEXT NOT NULL)`,
		`CREATE TABLE connection_profiles(id TEXT PRIMARY KEY, revision INTEGER NOT NULL, metadata TEXT NOT NULL)`,
		`CREATE TABLE deleted_connection_profiles(id TEXT PRIMARY KEY)`,
		`CREATE TABLE history_purpose(workspace_id TEXT NOT NULL REFERENCES workspaces(id),episode_id TEXT NOT NULL,purpose TEXT NOT NULL,updated_at TEXT NOT NULL,PRIMARY KEY(workspace_id,episode_id))`,
		`CREATE TABLE history_feedback(id TEXT PRIMARY KEY,workspace_id TEXT NOT NULL REFERENCES workspaces(id),episode_id TEXT NOT NULL,rating INTEGER NOT NULL,at TEXT NOT NULL)`,
		`CREATE TABLE workspace_events(workspace_id TEXT NOT NULL REFERENCES workspaces(id), project_id TEXT NOT NULL,id TEXT NOT NULL,at TEXT NOT NULL,body TEXT NOT NULL,PRIMARY KEY(workspace_id,id))`,
		`CREATE INDEX workspace_events_project_time ON workspace_events(project_id,at)`,
		`CREATE TABLE hermes_setups(project_id TEXT PRIMARY KEY REFERENCES projects(id),revision INTEGER NOT NULL,metadata TEXT NOT NULL)`,
		`CREATE TABLE forge_connections(id TEXT PRIMARY KEY, metadata TEXT NOT NULL)`,
		`CREATE TABLE forge_accounts(name TEXT PRIMARY KEY, revision INTEGER NOT NULL, metadata TEXT NOT NULL, credential TEXT NOT NULL)`,
		`CREATE TABLE board_items(id TEXT PRIMARY KEY,project_id TEXT NOT NULL,revision INTEGER NOT NULL,metadata TEXT NOT NULL)`,
		`CREATE TABLE work_review_history(id TEXT PRIMARY KEY,project_id TEXT NOT NULL,work_item_id TEXT NOT NULL,attempt_id TEXT NOT NULL,decision TEXT NOT NULL,at TEXT NOT NULL)`,
		`CREATE TABLE agent_subscriptions(owner TEXT NOT NULL,id TEXT NOT NULL,name TEXT NOT NULL,provider TEXT NOT NULL,credential TEXT NOT NULL,PRIMARY KEY(owner,id))`,
		`CREATE TABLE worker_agent_subscriptions(workspace_id TEXT NOT NULL,provider TEXT NOT NULL,owner TEXT NOT NULL,subscription_id TEXT NOT NULL,PRIMARY KEY(workspace_id,provider))`,
		`CREATE TABLE project_members(project_id TEXT NOT NULL,email TEXT NOT NULL,user_id TEXT NOT NULL,role TEXT NOT NULL,status TEXT NOT NULL,revision INTEGER NOT NULL,PRIMARY KEY(project_id,email))`,
		`CREATE TABLE work_references(project_id TEXT NOT NULL,work_id TEXT NOT NULL,number INTEGER NOT NULL,PRIMARY KEY(project_id,work_id),UNIQUE(project_id,number))`,
		`CREATE TABLE project_coordination(project_id TEXT PRIMARY KEY,workspace_id TEXT NOT NULL,notes_enabled INTEGER NOT NULL DEFAULT 0)`,
	} {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	for _, schema := range []string{orchestration.Schema, coordination.Schema, coordination.ConsumptionSchema} {
		for _, q := range strings.Split(schema, ";") {
			if strings.TrimSpace(q) == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx, q); err != nil {
				return err
			}
		}
	}
	for _, profile := range shared.DefaultConnectionProfiles() {
		metadata, err := json.Marshal(profile)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO connection_profiles(id,revision,metadata) VALUES($1,$2,$3)`, profile.ID, profile.Revision, string(metadata)); err != nil {
			return err
		}
	}
	return nil
}
