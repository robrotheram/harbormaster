package store

import (
	"agent-workbench/internal/shared"
	"context"
	"time"
)

type HistoryFeedback struct {
	ID        string    `json:"id"`
	EpisodeID string    `json:"episode_id"`
	Rating    int       `json:"rating"`
	At        time.Time `json:"at"`
	Reviewer  string    `json:"reviewer"`
}

func (s *Store) initHistoryFeedback(ctx context.Context) error {
	_, e := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS history_feedback(id TEXT PRIMARY KEY,workspace_id TEXT NOT NULL REFERENCES workspaces(id),episode_id TEXT NOT NULL,rating INTEGER NOT NULL,at TEXT NOT NULL)`)
	return e
}
func (s *Store) SaveHistoryFeedback(ctx context.Context, workspace, episode string, rating int) (HistoryFeedback, error) {
	v := HistoryFeedback{ID: shared.NewID("review_"), EpisodeID: episode, Rating: rating, At: time.Now().UTC(), Reviewer: "local-operator"}
	_, e := s.DB.ExecContext(ctx, s.query(`INSERT INTO history_feedback(id,workspace_id,episode_id,rating,at) VALUES(?,?,?,?,?)`), v.ID, workspace, episode, rating, v.At.Format(time.RFC3339Nano))
	return v, e
}
func (s *Store) HistoryFeedback(ctx context.Context, workspace string) (map[string]HistoryFeedback, error) {
	rows, e := s.DB.QueryContext(ctx, s.query(`SELECT id,episode_id,rating,at FROM history_feedback WHERE workspace_id=? ORDER BY at,id`), workspace)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := map[string]HistoryFeedback{}
	for rows.Next() {
		var v HistoryFeedback
		var at string
		if e = rows.Scan(&v.ID, &v.EpisodeID, &v.Rating, &at); e != nil {
			return nil, e
		}
		v.At, _ = time.Parse(time.RFC3339Nano, at)
		v.Reviewer = "local-operator"
		out[v.EpisodeID] = v
	}
	return out, rows.Err()
}
