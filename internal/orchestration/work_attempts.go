package orchestration

import (
	"agent-workbench/internal/shared"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// InitWorkAttempts is additive and safe to invoke from the startup migration owner.
func (r Repository) InitWorkAttempts(ctx context.Context) error {
	_, err := r.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS work_attempts(id TEXT PRIMARY KEY, work_item_id TEXT NOT NULL, project_id TEXT NOT NULL, workspace_id TEXT NOT NULL, delivery_key TEXT NOT NULL, active_workspace TEXT UNIQUE, metadata TEXT NOT NULL, UNIQUE(work_item_id,delivery_key))`)
	return err
}
func (r Repository) Attempts(ctx context.Context, project, id string) ([]shared.WorkAttempt, error) {
	if _, err := r.Work(ctx, project, id); err != nil {
		return nil, err
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT metadata FROM work_attempts WHERE project_id=$1 AND work_item_id=$2 ORDER BY id`, project, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []shared.WorkAttempt{}
	for rows.Next() {
		var b string
		var a shared.WorkAttempt
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(b), &a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
func (r Repository) RunWork(ctx context.Context, project, id, key string, now time.Time) (shared.WorkAttempt, error) {
	if !shared.ValidKey(key) {
		return shared.WorkAttempt{}, ErrInvalidWork
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return shared.WorkAttempt{}, err
	}
	defer tx.Rollback()
	var b string
	var a shared.WorkAttempt
	err = tx.QueryRowContext(ctx, `SELECT metadata FROM work_attempts WHERE project_id=$1 AND work_item_id=$2 AND delivery_key=$3`, project, id, key).Scan(&b)
	if err == nil {
		err = json.Unmarshal([]byte(b), &a)
		return a, err
	}
	if err != sql.ErrNoRows {
		return a, err
	}
	var item shared.WorkItem
	if err = tx.QueryRowContext(ctx, `SELECT metadata FROM work_items WHERE project_id=$1 AND id=$2`, project, id).Scan(&b); err != nil {
		return a, err
	}
	if err = json.Unmarshal([]byte(b), &item); err != nil {
		return a, err
	}
	switch item.State {
	case "draft", "failed", "cancelled", "awaiting_review":
	default:
		return a, ErrConflict
	}
	a = shared.WorkAttempt{ID: shared.NewID("attempt_"), WorkItemID: id, WorkspaceID: item.WorkspaceID, State: "queued", Artifacts: []shared.WorkArtifact{}, CreatedAt: now.UTC(), UpdatedAt: now.UTC()}
	ab, _ := json.Marshal(a)
	if _, err = tx.ExecContext(ctx, `INSERT INTO work_attempts(id,work_item_id,project_id,workspace_id,delivery_key,active_workspace,metadata) VALUES($1,$2,$3,$4,$5,$4,$6)`, a.ID, id, project, item.WorkspaceID, key, string(ab)); err != nil {
		if workUniqueConflict(err) {
			return a, ErrConflict
		}
		return a, err
	}
	old := b
	item.LatestAttemptID = a.ID
	item.State = "queued"
	item.UpdatedAt = now.UTC()
	b2, _ := json.Marshal(item)
	res, err := tx.ExecContext(ctx, `UPDATE work_items SET metadata=$1 WHERE id=$2 AND project_id=$3 AND metadata=$4`, string(b2), id, project, old)
	if err != nil {
		return a, err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return a, ErrConflict
	}
	return a, tx.Commit()
}
func (r Repository) saveAttempt(ctx context.Context, project string, a shared.WorkAttempt) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var b string
	var item shared.WorkItem
	if err = tx.QueryRowContext(ctx, `SELECT metadata FROM work_items WHERE project_id=$1 AND id=$2`, project, a.WorkItemID).Scan(&b); err != nil {
		return err
	}
	if err = json.Unmarshal([]byte(b), &item); err != nil {
		return err
	}
	ab, _ := json.Marshal(a)
	var active any = a.WorkspaceID
	if a.State == "failed" || a.State == "cancelled" || a.State == "awaiting_review" {
		active = nil
	}
	if _, err = tx.ExecContext(ctx, `UPDATE work_attempts SET metadata=$1,active_workspace=$2 WHERE id=$3 AND project_id=$4`, string(ab), active, a.ID, project); err != nil {
		return err
	}
	if item.LatestAttemptID == a.ID {
		item.State = a.State
		if item.State == "dispatching" {
			item.State = "queued"
		}
		if item.State == "cancel_requested" {
			item.State = "running"
		}
		item.UpdatedAt = a.UpdatedAt
		ib, _ := json.Marshal(item)
		res, e := tx.ExecContext(ctx, `UPDATE work_items SET metadata=$1 WHERE id=$2 AND metadata=$3`, string(ib), item.ID, b)
		if e != nil {
			return e
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return ErrConflict
		}
	}
	return tx.Commit()
}
func (r Repository) ReviewWork(ctx context.Context, project, id string, accepted bool, now time.Time) (shared.WorkItem, error) {
	item, err := r.Work(ctx, project, id)
	if err != nil {
		return item, err
	}
	target := "failed"
	if accepted {
		target = "completed"
	}
	if item.State == target {
		return item, nil
	}
	if item.State != "awaiting_review" {
		return item, ErrConflict
	}
	old, _ := json.Marshal(item)
	item.State = target
	item.UpdatedAt = now.UTC()
	b, _ := json.Marshal(item)
	res, err := r.DB.ExecContext(ctx, `UPDATE work_items SET metadata=$1 WHERE id=$2 AND project_id=$3 AND metadata=$4`, string(b), id, project, string(old))
	if err != nil {
		return item, err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return item, ErrConflict
	}
	return item, nil
}
func (r Repository) CancelWork(ctx context.Context, project, id string, now time.Time) (shared.WorkItem, error) {
	item, err := r.Work(ctx, project, id)
	if err != nil {
		return item, err
	}
	if item.State == "draft" || item.State == "cancelled" {
		return r.CancelDraft(ctx, project, id, now)
	}
	attempts, err := r.Attempts(ctx, project, id)
	if err != nil {
		return item, err
	}
	for _, a := range attempts {
		if a.ID == item.LatestAttemptID && (shared.Active(a.State) || a.State == "dispatching" || a.State == "blocked") {
			a.State = "cancel_requested"
			a.UpdatedAt = now.UTC()
			err = r.saveAttempt(ctx, project, a)
			if err != nil {
				return item, err
			}
			return r.Work(ctx, project, id)
		}
	}
	return item, ErrConflict
}

func workUniqueConflict(err error) bool {
	var postgres interface{ SQLState() string }
	if errors.As(err, &postgres) && postgres.SQLState() == "23505" {
		return true
	}
	var sqlite interface{ Code() int }
	return errors.As(err, &sqlite) && (sqlite.Code() == 2067 || sqlite.Code() == 1555)
}
