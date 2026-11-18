package store

import (
	"agent-workbench/internal/coordination"
	"agent-workbench/internal/orchestration"
	"context"

	"fmt"
	"github.com/jackc/pgx/v5/stdlib"
	"strings"
)

// Migrations run before listeners or workers start. Existing prototype schemas
// are adopted by the idempotent initial version without rewriting private data.
func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY)`); err != nil {
		return err
	}
	steps := []struct {
		version int
		apply   func(context.Context) error
	}{
		{1, s.initMetadata}, {2, func(ctx context.Context) error { return (orchestration.Repository{DB: s.DB}).Init(ctx) }},
		{3, func(ctx context.Context) error { return (orchestration.Repository{DB: s.DB}).InitWork(ctx) }},
		{4, func(ctx context.Context) error {
			for _, query := range strings.Split(coordination.Schema, ";") {
				if strings.TrimSpace(query) != "" {
					if _, err := s.DB.ExecContext(ctx, query); err != nil {
						return err
					}
				}
			}
			_, err := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS project_coordination(project_id TEXT PRIMARY KEY,workspace_id TEXT NOT NULL,notes_enabled INTEGER NOT NULL DEFAULT 0)`)
			return err
		}},
		{5, func(ctx context.Context) error { return (orchestration.Repository{DB: s.DB}).InitWorkAttempts(ctx) }},
		{6, func(ctx context.Context) error {
			for _, q := range strings.Split(coordination.ConsumptionSchema, ";") {
				if strings.TrimSpace(q) != "" {
					if _, err := s.DB.ExecContext(ctx, q); err != nil {
						return err
					}
				}
			}
			return nil
		}},
		{7, func(ctx context.Context) error { return (orchestration.Repository{DB: s.DB}).InitWorkRequests(ctx) }},
		{8, func(ctx context.Context) error {
			_, err := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS deleted_resources(kind TEXT NOT NULL,id TEXT NOT NULL,PRIMARY KEY(kind,id))`)
			return err
		}},
		{9, s.initForges},
		{10, s.initForgeAccounts},
		{11, s.initBoard},
		{12, s.initWorkReviewHistory},
		{13, s.initSubscriptions},
		{14, s.initAgentSubscriptions},
		{15, s.initMembership},
		{16, s.initWorkReferences},
		{17, s.migrateWorkspaceConnections},
	}
	for _, step := range steps {
		var applied int
		if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations WHERE version=`+fmt.Sprint(step.version)).Scan(&applied); err != nil {
			return err
		}
		if applied != 0 {
			continue
		}
		if err := step.apply(ctx); err != nil {
			return fmt.Errorf("migration %d: %w", step.version, err)
		}
		if _, err := s.DB.ExecContext(ctx, s.query(`INSERT INTO schema_migrations(version) VALUES(?) ON CONFLICT(version) DO NOTHING`), step.version); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) migrateWorkspaceConnections(ctx context.Context) error {
	// Preserve the old opt-in only when there is no explicit policy.
	const policy = `{"mode":"allowlist","destinations":["github.com:443"]}`
	query := `UPDATE workspaces
		SET metadata = json_set(json_remove(metadata, '$.github_access'), '$.connections', json($1))
		WHERE json_type(metadata, '$.github_access') = 'true'
		  AND json_extract(metadata, '$.connections') IS NULL`
	if _, postgres := s.DB.Driver().(*stdlib.Driver); postgres {
		query = `UPDATE workspaces
			SET metadata = jsonb_set(metadata::jsonb - 'github_access', '{connections}', $1::jsonb)::text
			WHERE metadata::jsonb -> 'github_access' = 'true'::jsonb
			  AND metadata::jsonb ->> 'connections' IS NULL`
	}
	_, err := s.DB.ExecContext(ctx, query, policy)
	return err
}

func (s *Store) initMetadata(ctx context.Context) error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS deleted_resources(kind TEXT NOT NULL,id TEXT NOT NULL,PRIMARY KEY(kind,id))`,
		`CREATE TABLE IF NOT EXISTS workspaces (id TEXT PRIMARY KEY, create_key TEXT UNIQUE NOT NULL, metadata TEXT NOT NULL)`,
		// Historical schema retained for safe upgrades; no active profile readers or seeds.
		`CREATE TABLE IF NOT EXISTS tool_profiles(id TEXT PRIMARY KEY,revision INTEGER NOT NULL,metadata TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS task_metadata (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, metadata TEXT NOT NULL)`,
	} {
		if _, e := s.DB.ExecContext(ctx, q); e != nil {
			return e
		}
	}
	if e := s.initConnectionProfiles(ctx); e != nil {
		return e
	}
	if e := s.initDomains(ctx); e != nil {
		return e
	}
	if e := s.initHistoryPurpose(ctx); e != nil {
		return e
	}
	if e := s.initHistoryFeedback(ctx); e != nil {
		return e
	}
	if e := s.initEvents(ctx); e != nil {
		return e
	}
	if e := s.initHermes(ctx); e != nil {
		return e
	}
	return nil
}
