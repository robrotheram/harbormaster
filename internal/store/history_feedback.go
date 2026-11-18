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

func (s *Store) SaveHistoryFeedback(ctx context.Context, workspace, episode string, rating int) (HistoryFeedback, error) {
	v := HistoryFeedback{ID: shared.NewID("review_"), EpisodeID: episode, Rating: rating, At: time.Now().UTC(), Reviewer: "local-operator"}
	_, e := s.DB.ExecContext(ctx, `INSERT INTO history_feedback(id,workspace_id,episode_id,rating,at) VALUES($1,$2,$3,$4,$5)`, v.ID, workspace, episode, rating, v.At.Format(time.RFC3339Nano))
	return v, e
}
func (s *Store) HistoryFeedback(ctx context.Context, workspace string) (map[string]HistoryFeedback, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT id,episode_id,rating,at FROM history_feedback WHERE workspace_id=$1 ORDER BY at,id`, workspace)
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
