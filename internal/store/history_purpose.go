package store

import (
	"agent-workbench/internal/shared"
	"context"
	"errors"
	"time"
)

// Operator annotations describe a run's purpose without altering its transcript.
func (s *Store) SaveHistoryPurpose(ctx context.Context, workspace, episode, purpose string) error {
	if !shared.ValidID(workspace) || !shared.ValidID(episode) || (purpose != "test" && purpose != "normal") {
		return errors.New("invalid history purpose")
	}
	_, err := s.DB.ExecContext(ctx, s.query(`INSERT INTO history_purpose(workspace_id,episode_id,purpose,updated_at) VALUES(?,?,?,?) ON CONFLICT(workspace_id,episode_id) DO UPDATE SET purpose=excluded.purpose,updated_at=excluded.updated_at`), workspace, episode, purpose, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
func (s *Store) HistoryPurposes(ctx context.Context, workspace string) (map[string]string, error) {
	rows, err := s.DB.QueryContext(ctx, s.query(`SELECT episode_id,purpose FROM history_purpose WHERE workspace_id=?`), workspace)
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
