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
	err := s.DB.QueryRowContext(ctx, s.query(`SELECT metadata FROM projects WHERE id=?`), id).Scan(&b)
	if err != nil {
		return p, err
	}
	err = json.Unmarshal([]byte(b), &p)
	return p, err
}
func (s *Store) ProjectByKey(ctx context.Context, key string) (shared.Project, error) {
	var p shared.Project
	var b string
	err := s.DB.QueryRowContext(ctx, s.query(`SELECT metadata FROM projects WHERE create_key=?`), key).Scan(&b)
	if err != nil {
		return p, err
	}
	err = json.Unmarshal([]byte(b), &p)
	return p, err
}
func (s *Store) Projects(ctx context.Context) ([]shared.Project, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT metadata FROM projects ORDER BY id`)
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
	_, err = s.DB.ExecContext(ctx, s.query(`INSERT INTO projects(id,create_key,metadata) VALUES(?,?,?)`), p.ID, key, string(b))
	return err
}
func (s *Store) AgentHomes(ctx context.Context, project string) ([]shared.AgentHome, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT metadata FROM agent_homes ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []shared.AgentHome{}
	for rows.Next() {
		var h shared.AgentHome
		var b string
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(b), &h); err != nil {
			return nil, err
		}
		if project == "" || h.ProjectID == project {
			out = append(out, h)
		}
	}
	return out, rows.Err()
}
func (s *Store) initDomains(ctx context.Context) error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS projects(id TEXT PRIMARY KEY,create_key TEXT UNIQUE NOT NULL,metadata TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS agent_homes(id TEXT PRIMARY KEY,project_id TEXT NOT NULL,workspace_id TEXT UNIQUE NOT NULL,metadata TEXT NOT NULL)`,
	} {
		if _, err := s.DB.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return s.reconcileLegacyHomes(ctx)
}

func (s *Store) reconcileLegacyHomes(ctx context.Context) error {
	legacy := shared.Project{ID: "default", Name: "Legacy workspaces", Legacy: true, CreatedAt: time.Now().UTC()}
	b, _ := json.Marshal(legacy)
	if _, err := s.DB.ExecContext(ctx, s.query(`INSERT INTO projects(id,create_key,metadata) VALUES(?,?,?) ON CONFLICT(id) DO NOTHING`), legacy.ID, "legacy-default", string(b)); err != nil {
		return err
	}
	// Read the old metadata first; do not hold a rows connection while creating
	// homes because the SQLite development store deliberately uses one connection.
	workspaces, err := s.Workspaces(ctx)
	if err != nil {
		return err
	}
	for _, w := range workspaces {
		if _, err = s.Project(ctx, w.ProjectID); err != nil {
			return err
		}
		homeID := w.HomeID
		if homeID == "" {
			homeID = "legacy_" + w.ID
		}
		h := shared.AgentHome{ID: homeID, ProjectID: w.ProjectID, WorkspaceID: w.ID, Kind: w.AgentKind, CreatedAt: w.CreatedAt}
		b, _ := json.Marshal(h)
		if _, err = s.DB.ExecContext(ctx, s.query(`INSERT INTO agent_homes(id,project_id,workspace_id,metadata) VALUES(?,?,?,?) ON CONFLICT(workspace_id) DO NOTHING`), h.ID, h.ProjectID, h.WorkspaceID, string(b)); err != nil {
			return err
		}
	}
	return nil
}
func workspaceKey(project, key string) string { return "workspace:" + project + ":" + key }
func (s *Store) ByProjectKey(ctx context.Context, project, key string) (shared.Workspace, error) {
	w, err := s.ByKey(ctx, workspaceKey(project, key))
	if err == sql.ErrNoRows && project == "default" {
		w, err = s.ByKey(ctx, key)
	}
	if err == nil && w.ProjectID != project {
		return shared.Workspace{}, sql.ErrNoRows
	}
	return w, err
}

func (s *Store) createWorkspace(ctx context.Context, w shared.Workspace, key string) error {
	w = shared.NormalizeWorkspace(w)
	if !shared.ValidID(w.ProjectID) || !shared.ValidID(w.HomeID) || !shared.ValidAgentKind(w.AgentKind) {
		return ErrOwnership
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists string
	if err = tx.QueryRowContext(ctx, s.query(`SELECT id FROM projects WHERE id=?`), w.ProjectID).Scan(&exists); err != nil {
		return err
	}
	b, err := json.Marshal(w)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, s.query(`INSERT INTO workspaces(id,create_key,metadata) VALUES(?,?,?)`), w.ID, workspaceKey(w.ProjectID, key), string(b)); err != nil {
		return err
	}
	h := shared.AgentHome{ID: w.HomeID, ProjectID: w.ProjectID, WorkspaceID: w.ID, Kind: w.AgentKind, CreatedAt: w.CreatedAt}
	b, _ = json.Marshal(h)
	if _, err = tx.ExecContext(ctx, s.query(`INSERT INTO agent_homes(id,project_id,workspace_id,metadata) VALUES(?,?,?,?)`), h.ID, h.ProjectID, h.WorkspaceID, string(b)); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) saveWorkspace(ctx context.Context, w shared.Workspace) error {
	w = shared.NormalizeWorkspace(w)
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
	old = shared.NormalizeWorkspace(old)
	if old.ProjectID != w.ProjectID || old.HomeID != w.HomeID || old.AgentKind != w.AgentKind || old.ParentID != w.ParentID || old.TemplateID != w.TemplateID || old.ProfileID != w.ProfileID || old.Branch != w.Branch {
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
