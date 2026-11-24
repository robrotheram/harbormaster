package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"agent-workbench/internal/jsonmetadata"
	"agent-workbench/internal/shared"
)

var ErrOwnership = errors.New("immutable ownership mismatch")

func (s *Store) Project(ctx context.Context, id string) (shared.Project, error) {
	return jsonmetadata.Get[shared.Project](ctx, s.DB, `SELECT metadata FROM projects WHERE id=$1 AND id NOT IN (SELECT id FROM deleted_resources WHERE kind='project')`, id)
}
func (s *Store) ProjectByKey(ctx context.Context, key string) (shared.Project, error) {
	return jsonmetadata.Get[shared.Project](ctx, s.DB, `SELECT metadata FROM projects WHERE create_key=$1 AND id NOT IN (SELECT id FROM deleted_resources WHERE kind='project')`, key)
}
func (s *Store) Projects(ctx context.Context) ([]shared.Project, error) {
	return jsonmetadata.Query[shared.Project](ctx, s.DB, `SELECT metadata FROM projects WHERE id NOT IN (SELECT id FROM deleted_resources WHERE kind='project') ORDER BY id`)
}

// Count creation reservations as well as ready projects so retries cannot evade
// quota while a project namespace is still provisioning.
func (s *Store) CreatedProjectCount(ctx context.Context, userID string) (int, error) {
	prefix := userID + ":"
	var count int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM projects WHERE substr(create_key,1,$1)=$2 AND id NOT IN (SELECT id FROM deleted_resources WHERE kind='project')`, len(prefix), prefix).Scan(&count)
	return count, err
}

func (s *Store) CreateProject(ctx context.Context, p shared.Project, key string) error {
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO projects(id,create_key,metadata) VALUES($1,$2,$3)`, p.ID, key, string(b)); err != nil {
		return err
	}
	if p.Sample {
		if err = s.createSampleTasks(ctx, tx, p); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ensureDefaultProject provisions the project used by default API and MCP requests.
func (s *Store) ensureDefaultProject(ctx context.Context) error {
	project := shared.Project{ID: "default", Name: "Workspaces", CreatedAt: time.Now().UTC()}
	body, err := json.Marshal(project)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO projects(id,create_key,metadata) VALUES($1,$2,$3) ON CONFLICT(id) DO NOTHING`, project.ID, ":default-project", string(body))
	return err
}

func workspaceKey(project, key string) string { return "workspace:" + project + ":" + key }
func (s *Store) ByProjectKey(ctx context.Context, project, key string) (shared.Workspace, error) {
	w, err := s.ByKey(ctx, workspaceKey(project, key))
	if err == nil && w.ProjectID != project {
		return shared.Workspace{}, sql.ErrNoRows
	}
	return w, err
}

func (s *Store) createWorkspace(ctx context.Context, w shared.Workspace, key string) error {
	if !shared.ValidID(w.ProjectID) || !shared.ValidID(w.HomeID) || !shared.ValidAgentKind(w.AgentKind) {
		return ErrOwnership
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists string
	if err = tx.QueryRowContext(ctx, `SELECT id FROM projects WHERE id=$1 AND NOT EXISTS (SELECT 1 FROM deleted_resources WHERE kind='project' AND deleted_resources.id=projects.id)`, w.ProjectID).Scan(&exists); err != nil {
		return err
	}
	b, err := json.Marshal(workspaceRecord{Workspace: w, QuotaUser: w.QuotaUserID})
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO workspaces(id,create_key,metadata) VALUES($1,$2,$3)`, w.ID, workspaceKey(w.ProjectID, key), string(b)); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) saveWorkspace(ctx context.Context, w shared.Workspace) error {
	if !shared.ValidID(w.HomeID) {
		return ErrOwnership
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	old, err := jsonmetadata.Get[shared.Workspace](ctx, tx, `SELECT metadata FROM workspaces WHERE id=$1`, w.ID)
	if err != nil {
		return err
	}
	if old.ProjectID != w.ProjectID || old.HomeID != w.HomeID || old.AgentKind != w.AgentKind || old.ParentID != w.ParentID || old.TemplateID != w.TemplateID || old.Branch != w.Branch {
		return ErrOwnership
	}
	data, err := json.Marshal(workspaceRecord{Workspace: w, QuotaUser: w.QuotaUserID})
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE workspaces SET metadata=$1 WHERE id=$2`, string(data), w.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// SaveProjectNamespace only updates a live project's deployment projection.
func (s *Store) SaveProjectNamespace(ctx context.Context, p shared.Project) error {
	if p.DeploymentNamespace == "" || p.DeploymentNamespace != shared.ProjectNamespace(p.ID) {
		return ErrOwnership
	}
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	result, err := s.DB.ExecContext(ctx, `UPDATE projects SET metadata=$1 WHERE id=$2 AND id NOT IN (SELECT id FROM deleted_resources WHERE kind='project')`, string(b), p.ID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return sql.ErrNoRows
	}
	return err
}

// RenameProject changes only the display name. Callers serialize project mutations.
func (s *Store) RenameProject(ctx context.Context, id, name string) (shared.Project, error) {
	p, err := s.Project(ctx, id)
	if err != nil {
		return p, err
	}
	p.Name = name
	body, err := json.Marshal(p)
	if err != nil {
		return p, err
	}
	result, err := s.DB.ExecContext(ctx, `UPDATE projects SET metadata=$1 WHERE id=$2 AND id NOT IN (SELECT id FROM deleted_resources WHERE kind='project')`, string(body), id)
	if err != nil {
		return p, err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		err = sql.ErrNoRows
	}
	return p, err
}
