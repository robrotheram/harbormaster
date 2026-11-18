package orchestration

import (
	"context"
	"encoding/json"
	"time"

	"agent-workbench/internal/jsonmetadata"
	"agent-workbench/internal/shared"
)

func (r Repository) HasUnfinishedRuns(ctx context.Context, id string) (bool, error) {
	var count int
	err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM orchestration_runs WHERE schedule_id=$1 AND state IN ('queued','dispatching','running','cancel_requested','unknown')`, id).Scan(&count)
	return count > 0, err
}

// Keep the schedule metadata for accepted runs and history. Cancel only work
// that has not begun dispatch; uncertain/accepted work keeps its durable identity.
// The caller holds the same schedule lock as the dispatcher and due delivery.
func (r Repository) DeleteSchedule(ctx context.Context, s shared.Schedule, now time.Time) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	queued, err := jsonmetadata.Query[shared.Run](ctx, tx, `SELECT metadata FROM orchestration_runs WHERE schedule_id=$1 AND state='queued'`, s.ID)
	if err != nil {
		return err
	}
	for _, run := range queued {
		if run.TaskID != "" {
			continue // Already accepted by the worker, even if its task is queued.
		}
		run.State, run.UpdatedAt = "cancelled", now
		body, _ := json.Marshal(run)
		if _, err = tx.ExecContext(ctx, `UPDATE orchestration_runs SET metadata=$1,state='cancelled' WHERE id=$2`, string(body), run.ID); err != nil {
			return err
		}
	}
	s.Enabled, s.DeletedAt = false, &now
	s.Revision++
	body, _ := json.Marshal(s)
	result, err := tx.ExecContext(ctx, `UPDATE schedules SET metadata=$1 WHERE id=$2 AND project_id=$3`, string(body), s.ID, s.ProjectID)
	if err = updated(result, err); err != nil {
		return err
	}
	return tx.Commit()
}
