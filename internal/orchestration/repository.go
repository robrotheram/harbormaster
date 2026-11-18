// Package orchestration persists only scheduling and execution metadata.
// Instruction bodies and native agent state remain in workspace-owned storage.
package orchestration

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"agent-workbench/internal/shared"
)

type Repository struct{ DB *sql.DB }

func (r Repository) Schedules(ctx context.Context, project string) ([]shared.Schedule, error) {
	q := `SELECT metadata FROM schedules`
	args := []any{}
	if project != "" {
		q += ` WHERE project_id=$1`
		args = append(args, project)
	}
	q += ` ORDER BY id`
	rows, err := r.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []shared.Schedule{}
	for rows.Next() {
		var b string
		var s shared.Schedule
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(b), &s); err != nil {
			return nil, err
		}
		if s.DeletedAt == nil {
			out = append(out, s)
		}
	}
	return out, rows.Err()
}

func (r Repository) Schedule(ctx context.Context, id string) (shared.Schedule, error) {
	var b string
	var s shared.Schedule
	err := r.DB.QueryRowContext(ctx, `SELECT metadata FROM schedules WHERE id=$1`, id).Scan(&b)
	if err == nil {
		err = json.Unmarshal([]byte(b), &s)
	}
	return s, err
}

func sameSchedule(a, b shared.Schedule) bool {
	return a.ProjectID == b.ProjectID && a.WorkspaceID == b.WorkspaceID && a.Name == b.Name && a.InstructionPath == b.InstructionPath && a.Provider == b.Provider && a.RunAt.Equal(b.RunAt) && a.IntervalSeconds == b.IntervalSeconds
}

var ErrConflict = errors.New("IDEMPOTENCY_CONFLICT")

func (r Repository) CreateSchedule(ctx context.Context, s shared.Schedule, key string) (shared.Schedule, bool, error) {
	b, _ := json.Marshal(s)
	result, err := r.DB.ExecContext(ctx, `INSERT INTO schedules(id,project_id,create_key,metadata) VALUES($1,$2,$3,$4) ON CONFLICT(project_id,create_key) DO NOTHING`, s.ID, s.ProjectID, key, string(b))
	if err != nil {
		return s, false, err
	}
	n, _ := result.RowsAffected()
	if n > 0 {
		return s, true, nil
	}
	var raw string
	var existing shared.Schedule
	err = r.DB.QueryRowContext(ctx, `SELECT metadata FROM schedules WHERE project_id=$1 AND create_key=$2`, s.ProjectID, key).Scan(&raw)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &existing)
	}
	if err == nil && (existing.DeletedAt != nil || !sameSchedule(existing, s)) {
		err = ErrConflict
	}
	return existing, false, err
}

func (r Repository) SaveSchedule(ctx context.Context, s shared.Schedule) error {
	b, _ := json.Marshal(s)
	result, err := r.DB.ExecContext(ctx, `UPDATE schedules SET metadata=$1 WHERE id=$2 AND project_id=$3`, string(b), s.ID, s.ProjectID)
	return updated(result, err)
}

func Occurrence(s shared.Schedule, key string, now time.Time) shared.Run {
	h := sha256.Sum256([]byte(s.ProjectID + "\x00" + s.ID + "\x00" + key))
	return shared.Run{ID: "run_" + hex.EncodeToString(h[:16]), ProjectID: s.ProjectID, WorkspaceID: s.WorkspaceID, ScheduleID: s.ID, State: "queued", CreatedAt: now, UpdatedAt: now}
}

func (r Repository) Enqueue(ctx context.Context, s shared.Schedule, key string, now time.Time) (shared.Run, error) {
	run := Occurrence(s, key, now)
	b, _ := json.Marshal(run)
	_, err := r.DB.ExecContext(ctx, `INSERT INTO orchestration_runs(id,project_id,schedule_id,state,created_at,metadata) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(id) DO NOTHING`, run.ID, run.ProjectID, run.ScheduleID, run.State, run.CreatedAt.Format(time.RFC3339Nano), string(b))
	if err != nil {
		return run, err
	}
	var raw string
	err = r.DB.QueryRowContext(ctx, `SELECT metadata FROM orchestration_runs WHERE id=$1`, run.ID).Scan(&raw)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &run)
	}
	return run, err
}

func (r Repository) Runs(ctx context.Context, project string) ([]shared.Run, error) {
	return r.runs(ctx, project)
}

func (r Repository) runs(ctx context.Context, project string) ([]shared.Run, error) {
	q := `SELECT metadata FROM orchestration_runs`
	args := []any{}
	if project != "" {
		q += ` WHERE project_id=$1`
		args = append(args, project)
	}
	q += ` ORDER BY created_at DESC LIMIT 1000`
	rows, err := r.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []shared.Run{}
	for rows.Next() {
		var raw string
		var run shared.Run
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &run); err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

func (r Repository) SaveRun(ctx context.Context, run shared.Run) error {
	b, _ := json.Marshal(run)
	result, err := r.DB.ExecContext(ctx, `UPDATE orchestration_runs SET metadata=$1,state=$4 WHERE id=$2 AND project_id=$3`, string(b), run.ID, run.ProjectID, run.State)
	return updated(result, err)
}

func updated(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (r Repository) PendingRunsAfter(ctx context.Context, after string, limit int) ([]shared.Run, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT metadata FROM orchestration_runs WHERE state IN ('queued','dispatching','running','cancel_requested') AND id>$1 ORDER BY id LIMIT $2`, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []shared.Run{}
	for rows.Next() {
		var b string
		var run shared.Run
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(b), &run); err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

// DeliverDue atomically records an occurrence and advances its trigger. Dispatch
// remains at least once and uses the occurrence's durable workspace key.
func (r Repository) DeliverDue(ctx context.Context, id string, now time.Time) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var raw string
	var schedule shared.Schedule
	if err = tx.QueryRowContext(ctx, `SELECT metadata FROM schedules WHERE id=$1`, id).Scan(&raw); err != nil {
		return err
	}
	if err = json.Unmarshal([]byte(raw), &schedule); err != nil {
		return err
	}
	next, run := AdvanceDue(schedule, now)
	if run == nil {
		return tx.Commit()
	}
	body, err := json.Marshal(run)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO orchestration_runs(id,project_id,schedule_id,state,created_at,metadata) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(id) DO NOTHING`, run.ID, run.ProjectID, run.ScheduleID, run.State, run.CreatedAt.Format(time.RFC3339Nano), string(body)); err != nil {
		return err
	}
	body, err = json.Marshal(next)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE schedules SET metadata=$1 WHERE id=$2 AND project_id=$3`, string(body), next.ID, next.ProjectID)
	if err = updated(result, err); err != nil {
		return err
	}
	return tx.Commit()
}
