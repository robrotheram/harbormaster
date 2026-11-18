package store

import (
	"agent-workbench/internal/shared"
	"context"
	"errors"
	"time"
)

func (s *Store) initHistoryPurpose(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS history_purpose(workspace_id TEXT NOT NULL REFERENCES workspaces(id),episode_id TEXT NOT NULL,purpose TEXT NOT NULL,updated_at TEXT NOT NULL,PRIMARY KEY(workspace_id,episode_id))`)
	return err
}

// Operator annotations describe a run's purpose without altering its transcript.
func (s *Store) SaveHistoryPurpose(ctx context.Context, workspace, episode, purpose string) error {
	if !shared.ValidID(workspace) || !shared.ValidID(episode) || (purpose != "test" && purpose != "normal") {
		return errors.New("invalid history purpose")
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO history_purpose(workspace_id,episode_id,purpose,updated_at) VALUES($1,$2,$3,$4) ON CONFLICT(workspace_id,episode_id) DO UPDATE SET purpose=excluded.purpose,updated_at=excluded.updated_at`, workspace, episode, purpose, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
func (s *Store) HistoryPurposes(ctx context.Context, workspace string) (map[string]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT episode_id,purpose FROM history_purpose WHERE workspace_id=$1`, workspace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var episode, purpose string
		if err = rows.Scan(&episode, &purpose); err != nil {
			return nil, err
		}
		out[episode] = purpose
	}
	return out, rows.Err()
}
