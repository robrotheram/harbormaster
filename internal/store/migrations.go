package store

import (
	"agent-workbench/internal/coordination"
	"agent-workbench/internal/orchestration"
	"context"
	"fmt"
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
func (s *Store) initMetadata(ctx context.Context) error {
	for _, q := range []string{
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
