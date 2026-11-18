package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"agent-workbench/internal/shared"
)

var ErrOwnership = errors.New("immutable ownership mismatch")

func (s *Store) Project(ctx context.Context, id string) (shared.Project, error) {
	var p shared.Project
	var b string
	err := s.DB.QueryRowContext(ctx, s.query(`SELECT metadata FROM projects WHERE id=? AND id NOT IN (SELECT id FROM deleted_resources WHERE kind='project')`), id).Scan(&b)
	if err != nil {
		return p, err
	}
	err = json.Unmarshal([]byte(b), &p)
	return p, err
}
func (s *Store) ProjectByKey(ctx context.Context, key string) (shared.Project, error) {
	var p shared.Project
	var b string
	err := s.DB.QueryRowContext(ctx, s.query(`SELECT metadata FROM projects WHERE create_key=? AND id NOT IN (SELECT id FROM deleted_resources WHERE kind='project')`), key).Scan(&b)
	if err != nil {
		return p, err
	}
	err = json.Unmarshal([]byte(b), &p)
	return p, err
}
func (s *Store) Projects(ctx context.Context) ([]shared.Project, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT metadata FROM projects WHERE id NOT IN (SELECT id FROM deleted_resources WHERE kind='project') ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []shared.Project{}
	for rows.Next() {
		var p shared.Project
		var b string
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(b), &p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
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
	if _, err = tx.ExecContext(ctx, s.query(`INSERT INTO projects(id,create_key,metadata) VALUES(?,?,?)`), p.ID, key, string(b)); err != nil {
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
	_, err = s.DB.ExecContext(ctx, s.query(`INSERT INTO projects(id,create_key,metadata) VALUES(?,?,?) ON CONFLICT(id) DO NOTHING`), project.ID, ":default-project", string(body))
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
	if err = tx.QueryRowContext(ctx, s.query(`SELECT id FROM projects WHERE id=? AND NOT EXISTS (SELECT 1 FROM deleted_resources WHERE kind='project' AND deleted_resources.id=projects.id)`), w.ProjectID).Scan(&exists); err != nil {
		return err
	}
	b, err := json.Marshal(w)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, s.query(`INSERT INTO workspaces(id,create_key,metadata) VALUES(?,?,?)`), w.ID, workspaceKey(w.ProjectID, key), string(b)); err != nil {
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
	var b string
	if err = tx.QueryRowContext(ctx, s.query(`SELECT metadata FROM workspaces WHERE id=?`), w.ID).Scan(&b); err != nil {
		return err
	}
	var old shared.Workspace
	if err = json.Unmarshal([]byte(b), &old); err != nil {
		return err
	}
	if old.ProjectID != w.ProjectID || old.HomeID != w.HomeID || old.AgentKind != w.AgentKind || old.ParentID != w.ParentID || old.TemplateID != w.TemplateID || old.Branch != w.Branch {
		return ErrOwnership
	}
	data, err := json.Marshal(w)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, s.query(`UPDATE workspaces SET metadata=? WHERE id=?`), string(data), w.ID); err != nil {
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
	result, err := s.DB.ExecContext(ctx, s.query(`UPDATE projects SET metadata=? WHERE id=? AND id NOT IN (SELECT id FROM deleted_resources WHERE kind='project')`), string(b), p.ID)
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
	result, err := s.DB.ExecContext(ctx, s.query(`UPDATE projects SET metadata=? WHERE id=? AND id NOT IN (SELECT id FROM deleted_resources WHERE kind='project')`), string(body), id)
	if err != nil {
		return p, err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		err = sql.ErrNoRows
	}
	return p, err
}
