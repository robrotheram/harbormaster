// Package store persists platform metadata. Content belongs in workspace storage.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"agent-workbench/internal/shared"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

type Store struct {
	DB *sql.DB
}

func Open(ctx context.Context, dsn string) (*Store, error) {
	driver := "sqlite"
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		driver = "pgx"
	}
	db, e := sql.Open(driver, dsn)
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	s := &Store{DB: db}
	if e = db.PingContext(ctx); e != nil {
		db.Close()
		return nil, e
	}
	if e = s.migrate(ctx); e != nil {
		db.Close()
		return nil, e
	}
	if e = s.reconcileLegacyProjects(ctx); e != nil {
		db.Close()
		return nil, e
	}
	return s, nil
}
func (s *Store) Create(ctx context.Context, w shared.Workspace, key string) error {
	return s.createWorkspace(ctx, w, key)
}
func (s *Store) ByKey(ctx context.Context, key string) (shared.Workspace, error) {
	var b string
	var w shared.Workspace
	e := s.DB.QueryRowContext(ctx, `SELECT metadata FROM workspaces WHERE create_key=$1 AND id NOT IN (SELECT id FROM deleted_resources WHERE kind='workspace')`, key).Scan(&b)
	if e != nil {
		return w, e
	}
	e = json.Unmarshal([]byte(b), &w)
	return shared.NormalizeWorkspace(w), e
}
func (s *Store) Workspaces(ctx context.Context) ([]shared.Workspace, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT metadata FROM workspaces WHERE id NOT IN (SELECT id FROM deleted_resources WHERE kind='workspace') ORDER BY id`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []shared.Workspace{}
	for rows.Next() {
		var b string
		var w shared.Workspace
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(b), &w); e != nil {
			return nil, e
		}
		out = append(out, shared.NormalizeWorkspace(w))
	}
	return out, rows.Err()
}
func (s *Store) Workspace(ctx context.Context, id string) (shared.Workspace, error) {
	var b string
	var w shared.Workspace
	e := s.DB.QueryRowContext(ctx, `SELECT metadata FROM workspaces WHERE id=$1 AND id NOT IN (SELECT id FROM deleted_resources WHERE kind='workspace')`, id).Scan(&b)
	if e != nil {
		return w, e
	}
	e = json.Unmarshal([]byte(b), &w)
	return shared.NormalizeWorkspace(w), e
}
func (s *Store) Save(ctx context.Context, w shared.Workspace) error { return s.saveWorkspace(ctx, w) }
func (s *Store) SaveTask(ctx context.Context, t shared.Task) error {
	b, e := json.Marshal(t)
	if e != nil {
		return e
	}
	result, e := s.DB.ExecContext(ctx, `INSERT INTO task_metadata(id,workspace_id,metadata) VALUES($1,$2,$3) ON CONFLICT(id) DO UPDATE SET metadata=excluded.metadata WHERE task_metadata.workspace_id=excluded.workspace_id`, t.ID, t.WorkspaceID, string(b))
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return ErrOwnership
	}
	return nil
}
func (s *Store) Tasks(ctx context.Context) ([]shared.Task, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT metadata FROM task_metadata ORDER BY id`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []shared.Task{}
	for rows.Next() {
		var b string
		var t shared.Task
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(b), &t); e != nil {
			return nil, e
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
