// Package store persists platform metadata and the private transcript cache.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"agent-workbench/internal/jsonmetadata"
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
	if e = s.ensureDefaultProject(ctx); e != nil {
		db.Close()
		return nil, e
	}
	return s, nil
}
func (s *Store) Create(ctx context.Context, w shared.Workspace, key string) error {
	return s.createWorkspace(ctx, w, key)
}
func (s *Store) ByKey(ctx context.Context, key string) (shared.Workspace, error) {
	return s.workspaceMetadata(ctx, `SELECT metadata FROM workspaces WHERE create_key=$1 AND id NOT IN (SELECT id FROM deleted_resources WHERE kind='workspace')`, key)
}
func (s *Store) Workspaces(ctx context.Context) ([]shared.Workspace, error) {
	records, err := jsonmetadata.Query[workspaceRecord](ctx, s.DB, `SELECT metadata FROM workspaces WHERE id NOT IN (SELECT id FROM deleted_resources WHERE kind='workspace') ORDER BY id`)
	if err != nil {
		return nil, err
	}
	workspaces := make([]shared.Workspace, 0, len(records))
	for _, record := range records {
		workspaces = append(workspaces, record.workspace())
	}
	return workspaces, nil
}
func (s *Store) Workspace(ctx context.Context, id string) (shared.Workspace, error) {
	return s.workspaceMetadata(ctx, `SELECT metadata FROM workspaces WHERE id=$1 AND id NOT IN (SELECT id FROM deleted_resources WHERE kind='workspace')`, id)
}

// Workspace defaults apply after a successful scan, including on decode errors.
func (s *Store) workspaceMetadata(ctx context.Context, query string, args ...any) (shared.Workspace, error) {
	var metadata string
	if err := s.DB.QueryRowContext(ctx, query, args...).Scan(&metadata); err != nil {
		return shared.Workspace{}, err
	}
	var record workspaceRecord
	err := json.Unmarshal([]byte(metadata), &record)
	return record.workspace(), err
}

type workspaceRecord struct {
	shared.Workspace
	QuotaUser string `json:"quota_user_id,omitempty"`
}

func (r workspaceRecord) workspace() shared.Workspace {
	r.Workspace.QuotaUserID = r.QuotaUser
	return r.Workspace
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
	return jsonmetadata.Query[shared.Task](ctx, s.DB, `SELECT metadata FROM task_metadata ORDER BY id`)
}
