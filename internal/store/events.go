package store

import (
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"fmt"
)

func (s *Store) initEvents(ctx context.Context) error {
	_, e := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS workspace_events(workspace_id TEXT NOT NULL REFERENCES workspaces(id), project_id TEXT NOT NULL,id TEXT NOT NULL,at TEXT NOT NULL,body TEXT NOT NULL,PRIMARY KEY(workspace_id,id))`)
	if e == nil {
		_, e = s.DB.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS workspace_events_project_time ON workspace_events(project_id,at)`)
	}
	return e
}
func (s *Store) SaveEvents(ctx context.Context, w shared.Workspace, events []shared.ActivityEvent) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	for _, v := range events {
		if !shared.SafeActivityEvent(v) {
			continue
		}
		v.At = v.At.UTC()
		b, _ := json.Marshal(v)
		if _, e = tx.ExecContext(ctx, `INSERT INTO workspace_events(workspace_id,project_id,id,at,body) VALUES($1,$2,$3,$4,$5) ON CONFLICT(workspace_id,id) DO NOTHING`, w.ID, w.ProjectID, v.ID, v.At.Format("2006-01-02T15:04:05.000000000Z07:00"), string(b)); e != nil {
			return e
		}
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM workspace_events WHERE workspace_id=$1 AND id NOT IN (SELECT id FROM workspace_events WHERE workspace_id=$2 ORDER BY at DESC LIMIT 2000)`, w.ID, w.ID); e != nil {
		return e
	}
	return tx.Commit()
}

type WorkspaceEvent struct {
	shared.ActivityEvent
	WorkspaceID string `json:"workspace_id"`
	ProjectID   string `json:"project_id"`
}

func (s *Store) Events(ctx context.Context, project, workspace string) ([]WorkspaceEvent, error) {
	q := `SELECT workspace_id,project_id,body FROM workspace_events WHERE 1=1`
	args := []any{}
	if project != "" {
		q += ` AND project_id=$1`
		args = append(args, project)
	}
	if workspace != "" {
		q += fmt.Sprintf(` AND workspace_id=$%d`, len(args)+1)
		args = append(args, workspace)
	}
	rows, e := s.DB.QueryContext(ctx, q+` ORDER BY at DESC LIMIT 500`, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []WorkspaceEvent{}
	for rows.Next() {
		var v WorkspaceEvent
		var b string
		if e = rows.Scan(&v.WorkspaceID, &v.ProjectID, &b); e != nil {
			return nil, e
		}
		if json.Unmarshal([]byte(b), &v.ActivityEvent) == nil {
			out = append(out, v)
		}
	}
	return out, rows.Err()
}
