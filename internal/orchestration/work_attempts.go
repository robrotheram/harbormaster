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
	return r.runWork(ctx, project, id, key, nil, now)
}

// WorkRevision contains private file references, never conversational content.
type WorkRevision struct {
	ExpectedAttemptID, Path, Digest string
	Selection                       *shared.AgentModel
}

func (r Repository) ContinueWork(ctx context.Context, project, id, key string, revision WorkRevision, now time.Time) (shared.WorkAttempt, error) {
	if (revision.Selection != nil && !shared.ValidWorkSelection(revision.Selection.Provider, revision.Selection.Model)) || !shared.ValidID(revision.ExpectedAttemptID) || !shared.ValidProjectFile(revision.Path) || len(revision.Digest) != 64 {
		return shared.WorkAttempt{}, ErrInvalidWork
	}
	return r.runWork(ctx, project, id, key, &revision, now)
}

func (r Repository) runWork(ctx context.Context, project, id, key string, revision *WorkRevision, now time.Time) (shared.WorkAttempt, error) {
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
		if err == nil && revision != nil && (a.ParentAttemptID != revision.ExpectedAttemptID || a.SpecificationPath != revision.Path || a.SpecificationDigest != revision.Digest || (revision.Selection != nil && (a.Provider != revision.Selection.Provider || a.Model != revision.Selection.Model))) {
			return a, ErrConflict
		}
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
	// Planning-only items have no executable specification or worker yet.
	if item.WorkspaceID == "" || item.SpecificationPath == "" || item.SpecificationDigest == "" {
		return a, ErrConflict
	}
	if !workActorAllowed(ctx, item) {
		return a, ErrConflict
	}
	if revision != nil && item.LatestAttemptID != revision.ExpectedAttemptID {
		return a, ErrConflict
	}
	switch item.State {
	case "draft", "planned", "in_progress", "changes_requested", "failed", "cancelled", "awaiting_review":
	case "completed":
		if revision == nil {
			return a, ErrConflict
		}
	default:
		return a, ErrConflict
	}
	old := b
	if revision != nil {
		// Older attempts predate specification snapshots. Retain their original
		// references before advancing the parent to its next instruction.
		var prior shared.WorkAttempt
		if err = tx.QueryRowContext(ctx, `SELECT metadata FROM work_attempts WHERE id=$1 AND work_item_id=$2 AND project_id=$3`, item.LatestAttemptID, id, project).Scan(&b); err != nil {
			return a, err
		}
		if err = json.Unmarshal([]byte(b), &prior); err != nil {
			return a, err
		}
		if prior.SpecificationPath == "" {
			prior.SpecificationPath, prior.SpecificationDigest = item.SpecificationPath, item.SpecificationDigest
		}
		if item.State == "completed" {
			prior.ReviewState = "accepted"
		} else if item.State == "awaiting_review" {
			prior.ReviewState = "changes_requested"
		}
		if !prior.SelectionRecorded {
			prior.Provider, prior.Model, prior.SelectionRecorded = item.Provider, item.Model, true
		}
		pb, _ := json.Marshal(prior)
		if _, err = tx.ExecContext(ctx, `UPDATE work_attempts SET metadata=$1 WHERE id=$2 AND project_id=$3`, string(pb), prior.ID, project); err != nil {
			return a, err
		}
		item.SpecificationPath, item.SpecificationDigest = revision.Path, revision.Digest
		if revision.Selection != nil {
			item.Provider, item.Model = revision.Selection.Provider, revision.Selection.Model
		}
	}
	a = shared.WorkAttempt{Provider: item.Provider, Model: item.Model, SelectionRecorded: true, ID: shared.NewID("attempt_"), WorkItemID: id, WorkspaceID: item.WorkspaceID, SpecificationPath: item.SpecificationPath, SpecificationDigest: item.SpecificationDigest, State: "queued", Artifacts: []shared.WorkArtifact{}, CreatedAt: now.UTC(), UpdatedAt: now.UTC()}
	if revision != nil {
		a.ParentAttemptID = revision.ExpectedAttemptID
	}
	ab, _ := json.Marshal(a)
	if _, err = tx.ExecContext(ctx, `INSERT INTO work_attempts(id,work_item_id,project_id,workspace_id,delivery_key,active_workspace,metadata) VALUES($1,$2,$3,$4,$5,$4,$6)`, a.ID, id, project, item.WorkspaceID, key, string(ab)); err != nil {
		if workUniqueConflict(err) {
			return a, ErrConflict
		}
		return a, err
	}
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

// ReviewRevisionBy checks the actor and reviewed revision in the same snapshot
// used by the compare-and-swap update.
func (r Repository) ReviewRevisionBy(ctx context.Context, project, id, decision, expected, actor string, now time.Time) (shared.WorkItem, error) {
	if expected == "" || (decision != "accepted" && decision != "changes_requested" && decision != "closed") {
		return shared.WorkItem{}, ErrInvalidWork
	}
	item, err := r.Work(ctx, project, id)
	if err != nil {
		return item, err
	}
	if actor != "" && item.ReviewerID != "" && item.ReviewerID != actor {
		return item, ErrConflict
	}
	if item.LatestAttemptID != expected {
		return item, ErrConflict
	}
	target := "changes_requested"
	if decision == "accepted" {
		target = "completed"
	}
	if decision == "closed" {
		target = "closed"
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
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return item, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE work_items SET metadata=$1 WHERE id=$2 AND project_id=$3 AND metadata=$4`, string(b), id, project, string(old))
	if err != nil {
		return item, err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return item, ErrConflict
	}
	if item.LatestAttemptID != "" {
		var raw string
		if err = tx.QueryRowContext(ctx, `SELECT metadata FROM work_attempts WHERE id=$1 AND project_id=$2 AND work_item_id=$3`, item.LatestAttemptID, project, id).Scan(&raw); err != nil {
			return item, err
		}
		var attempt shared.WorkAttempt
		if err = json.Unmarshal([]byte(raw), &attempt); err != nil {
			return item, err
		}
		attempt.ReviewState = decision
		attempt.ReviewedBy = actor
		attempt.UpdatedAt = now.UTC()
		data, _ := json.Marshal(attempt)
		if _, err = tx.ExecContext(ctx, `UPDATE work_attempts SET metadata=$1 WHERE id=$2 AND project_id=$3`, string(data), attempt.ID, project); err != nil {
			return item, err
		}
	}
	if err = tx.Commit(); err != nil {
		return item, err
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
