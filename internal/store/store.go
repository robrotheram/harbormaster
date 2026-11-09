// Package store persists platform metadata. Content belongs in workspace storage.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"agent-workbench/internal/shared"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

type Store struct {
	DB       *sql.DB
	postgres bool
}

func Open(ctx context.Context, dsn string) (*Store, error) {
	driver := "sqlite"
	pg := strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://")
	if pg {
		driver = "pgx"
	}
	db, e := sql.Open(driver, dsn)
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	s := &Store{db, pg}
	if e = db.PingContext(ctx); e != nil {
		db.Close()
		return nil, e
	}
	if e = s.migrate(ctx); e != nil {
		db.Close()
		return nil, e
	}
	if e = s.reconcileLegacyHomes(ctx); e != nil {
		db.Close()
		return nil, e
	}
	return s, nil
}
func (s *Store) query(q string) string {
	if !s.postgres {
		return q
	}
	n := 0
	return replaceQuestions(q, &n)
}
func replaceQuestions(q string, n *int) string {
	var b strings.Builder
	for _, r := range q {
		if r == '?' {
			*n++
			fmt.Fprintf(&b, "$%d", *n)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
func (s *Store) Create(ctx context.Context, w shared.Workspace, key string) error {
	return s.createWorkspace(ctx, w, key)
}
func (s *Store) ByKey(ctx context.Context, key string) (shared.Workspace, error) {
	var b string
	var w shared.Workspace
	e := s.DB.QueryRowContext(ctx, s.query(`SELECT metadata FROM workspaces WHERE create_key=?`), key).Scan(&b)
	if e != nil {
		return w, e
	}
	e = json.Unmarshal([]byte(b), &w)
	return shared.NormalizeWorkspace(w), e
}
func (s *Store) Workspaces(ctx context.Context) ([]shared.Workspace, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT metadata FROM workspaces ORDER BY id`)
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
	e := s.DB.QueryRowContext(ctx, s.query(`SELECT metadata FROM workspaces WHERE id=?`), id).Scan(&b)
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
	result, e := s.DB.ExecContext(ctx, s.query(`INSERT INTO task_metadata(id,workspace_id,metadata) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET metadata=excluded.metadata WHERE task_metadata.workspace_id=excluded.workspace_id`), t.ID, t.WorkspaceID, string(b))
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
