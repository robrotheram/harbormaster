package store

import (
	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"time"
)

func (s *Store) initWorkReviewHistory(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS work_review_history(id TEXT PRIMARY KEY,project_id TEXT NOT NULL,work_item_id TEXT NOT NULL,attempt_id TEXT NOT NULL,decision TEXT NOT NULL,at TEXT NOT NULL)`)
	return err
}

// MoveBoardWork changes planning and task status together. It retains the last
// decision, clears the current review, and never creates or dispatches an attempt.
func (s *Store) MoveBoardWork(ctx context.Context, board BoardItem, expected int, previous shared.WorkItem, now time.Time) (shared.WorkItem, error) {
	target := map[string]string{"planned": "planned", "running": "in_progress", "review": "awaiting_review", "done": "closed"}[board.Stage]
	if target == "" || !(BoardStage(previous.State) == "done" || previous.State == "planned" || previous.State == "in_progress" || previous.State == "awaiting_review") {
		return previous, orchestration.ErrConflict
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return previous, err
	}
	defer tx.Rollback()
	var raw string
	if err = tx.QueryRowContext(ctx, s.query(`SELECT metadata FROM work_items WHERE project_id=? AND id=?`), previous.ProjectID, previous.ID).Scan(&raw); err != nil {
		return previous, err
	}
	var work shared.WorkItem
	if err = json.Unmarshal([]byte(raw), &work); err != nil {
		return previous, err
	}
	if work.State != previous.State || work.LatestAttemptID != previous.LatestAttemptID || !work.UpdatedAt.Equal(previous.UpdatedAt) {
		return previous, orchestration.ErrConflict
	}
	if work.LatestAttemptID != "" {
		var prior string
		if err = tx.QueryRowContext(ctx, s.query(`SELECT metadata FROM work_attempts WHERE project_id=? AND work_item_id=? AND id=? AND active_workspace IS NULL`), work.ProjectID, work.ID, work.LatestAttemptID).Scan(&prior); err != nil {
			return previous, err
		}
		var attempt shared.WorkAttempt
		if err = json.Unmarshal([]byte(prior), &attempt); err != nil {
			return previous, err
		}
		decision := attempt.ReviewState
		if decision == "" && BoardStage(previous.State) == "done" {
			decision = "accepted"
			if work.State == "closed" {
				decision = "closed"
			}
		}

		if decision != "" {
			if _, err = tx.ExecContext(ctx, s.query(`INSERT INTO work_review_history(id,project_id,work_item_id,attempt_id,decision,at) VALUES(?,?,?,?,?,?)`), shared.NewID("review_"), work.ProjectID, work.ID, attempt.ID, decision, now.UTC().Format(time.RFC3339Nano)); err != nil {
				return previous, err
			}
		}
		attempt.ReviewState = ""
		if target == "closed" {
			attempt.ReviewState = "closed"
		}
		attempt.UpdatedAt = now.UTC()
		data, _ := json.Marshal(attempt)
		result, e := tx.ExecContext(ctx, s.query(`UPDATE work_attempts SET metadata=? WHERE project_id=? AND id=? AND metadata=? AND active_workspace IS NULL`), string(data), work.ProjectID, attempt.ID, prior)
		if e != nil {
			return previous, e
		}
		if n, _ := result.RowsAffected(); n != 1 {
			return previous, orchestration.ErrConflict
		}
	}
	work.State = target
	work.UpdatedAt = now.UTC()
	data, _ := json.Marshal(work)
	result, err := tx.ExecContext(ctx, s.query(`UPDATE work_items SET metadata=? WHERE project_id=? AND id=? AND metadata=?`), string(data), work.ProjectID, work.ID, raw)
	if err != nil {
		return previous, err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return previous, orchestration.ErrConflict
	}
	board.ExecutionState = target
	if err = s.saveBoardItem(ctx, tx, board, expected); err != nil {
		return previous, err
	}
	return work, tx.Commit()
}
