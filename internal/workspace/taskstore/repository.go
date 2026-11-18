// Package taskstore owns durable task admission, execution state and recovery.
// Instructions and results remain in the private workspace database.
package taskstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"agent-workbench/internal/shared"
)

var ErrConflict = errors.New("IDEMPOTENCY_CONFLICT")
var ErrQueueFull = errors.New("QUEUE_FULL")

// Repository borrows a workspace-owned SQLite database. The supervisor holds
// the persistent-home writer lock for this database's entire lifetime.
type Repository struct {
	db        *sql.DB
	workspace string
}

func New(db *sql.DB, workspace string) *Repository { return &Repository{db: db, workspace: workspace} }

func (r *Repository) Init(ctx context.Context) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS tasks(id TEXT PRIMARY KEY,idem_key TEXT UNIQUE NOT NULL,fingerprint TEXT NOT NULL,instruction TEXT NOT NULL,state TEXT NOT NULL,result TEXT NOT NULL DEFAULT '',messages TEXT NOT NULL DEFAULT '[]',error_code TEXT NOT NULL DEFAULT '',created_at TEXT NOT NULL,updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS file_acceptances(idem_key TEXT PRIMARY KEY,instruction_path TEXT NOT NULL,provider TEXT NOT NULL,task_id TEXT NOT NULL)`,
		`UPDATE tasks SET state='failed',error_code='INTERRUPTED',updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE state IN ('queued','running','cancel_requested')`,
	} {
		if _, err = tx.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	// Additive migration: old receipts retain the empty digest contract.
	var count, modelCount int
	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(file_acceptances)`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var cid, nn, pk int
		var name, typ string
		var def any
		if err := rows.Scan(&cid, &name, &typ, &nn, &def, &pk); err != nil {
			rows.Close()
			return err
		}
		if name == "model" {
			modelCount++
		}
		if name == "specification_digest" {
			count++
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if count == 0 {
		if _, err := tx.ExecContext(ctx, `ALTER TABLE file_acceptances ADD COLUMN specification_digest TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	if modelCount == 0 {
		if _, err := tx.ExecContext(ctx, `ALTER TABLE file_acceptances ADD COLUMN model TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type Submission struct{ Instruction, Key, Provider, Model, SourcePath, SpecificationDigest string }
type Acceptance struct {
	Task    shared.Task
	Created bool
}

type scanner interface{ Scan(...any) error }

func (r *Repository) scan(row scanner) (shared.Task, error) {
	task := shared.Task{WorkspaceID: r.workspace}
	var created, updated string
	err := row.Scan(&task.ID, &task.State, &task.ErrorCode, &created, &updated)
	if err != nil {
		return task, err
	}
	task.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return task, err
	}
	task.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	return task, err
}
func (r *Repository) Get(ctx context.Context, id string) (shared.Task, error) {
	return r.scan(r.db.QueryRowContext(ctx, `SELECT id,state,error_code,created_at,updated_at FROM tasks WHERE id=?`, id))
}
func (r *Repository) FileAcceptance(ctx context.Context, key, path, provider, digest, model string) (shared.Task, error) {
	var oldPath, oldProvider, id, oldDigest, oldModel string
	if err := r.db.QueryRowContext(ctx, `SELECT instruction_path,provider,task_id,specification_digest,model FROM file_acceptances WHERE idem_key=?`, key).Scan(&oldPath, &oldProvider, &id, &oldDigest, &oldModel); err != nil {
		return shared.Task{}, err
	}
	if model != oldModel || path != oldPath || provider != oldProvider || digest != oldDigest {
		return shared.Task{}, ErrConflict
	}
	return r.Get(ctx, id)
}

// Accept commits the idempotency record, optional file acceptance and queue
// reservation atomically, before any execution goroutine can be started.
func (r *Repository) Accept(ctx context.Context, s Submission, now time.Time) (Acceptance, error) {
	input := s.Instruction
	if s.SourcePath != "" {
		input = "file:" + s.SourcePath + "\x00" + input
	}
	if s.Provider != "" {
		input += "\x00" + s.Provider
	}
	if s.Model != "" {
		input += "\x00model:" + s.Model
	}
	sum := sha256.Sum256([]byte(input))
	fingerprint := hex.EncodeToString(sum[:])
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Acceptance{}, err
	}
	defer tx.Rollback()
	var id, previous string
	err = tx.QueryRowContext(ctx, `SELECT id,fingerprint FROM tasks WHERE idem_key=?`, s.Key).Scan(&id, &previous)
	if err == nil {
		if previous != fingerprint {
			return Acceptance{}, ErrConflict
		}
		task, err := r.scan(tx.QueryRowContext(ctx, `SELECT id,state,error_code,created_at,updated_at FROM tasks WHERE id=?`, id))
		return Acceptance{Task: task}, err
	}
	if err != sql.ErrNoRows {
		return Acceptance{}, err
	}
	var active int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM tasks WHERE state IN ('queued','running','cancel_requested')`).Scan(&active); err != nil {
		return Acceptance{}, err
	}
	if active >= 16 {
		return Acceptance{}, ErrQueueFull
	}
	id = shared.NewID("task_")
	if _, err = tx.ExecContext(ctx, `INSERT INTO tasks(id,idem_key,fingerprint,instruction,state,created_at,updated_at) VALUES(?,?,?,?,'queued',?,?)`, id, s.Key, fingerprint, s.Instruction, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		return Acceptance{}, err
	}
	if s.SourcePath != "" {
		if _, err = tx.ExecContext(ctx, `INSERT INTO file_acceptances(idem_key,instruction_path,provider,task_id,specification_digest,model) VALUES(?,?,?,?,?,?)`, s.Key, s.SourcePath, s.Provider, id, s.SpecificationDigest, s.Model); err != nil {
			return Acceptance{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return Acceptance{}, err
	}
	return Acceptance{Created: true, Task: shared.Task{ID: id, WorkspaceID: r.workspace, State: "queued", CreatedAt: now, UpdatedAt: now}}, nil
}
func (r *Repository) List(ctx context.Context) ([]shared.Task, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,state,error_code,created_at,updated_at FROM tasks ORDER BY created_at DESC LIMIT 1000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []shared.Task{}
	for rows.Next() {
		task, err := r.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, task)
	}
	return out, rows.Err()
}
func (r *Repository) ActiveCount(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM tasks WHERE state IN ('queued','running','cancel_requested')`).Scan(&n)
	return n, err
}
func (r *Repository) Start(ctx context.Context, id string, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE tasks SET state='running',updated_at=? WHERE id=?`, now.Format(time.RFC3339Nano), id)
	return err
}
func (r *Repository) Finish(ctx context.Context, id, state, result, code string, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE tasks SET state=?,result=?,error_code=?,updated_at=? WHERE id=?`, state, result, code, now.Format(time.RFC3339Nano), id)
	return err
}
func (r *Repository) SaveMessages(ctx context.Context, id string, body []byte) error {
	_, err := r.db.ExecContext(ctx, `UPDATE tasks SET messages=? WHERE id=?`, string(body), id)
	return err
}
func (r *Repository) RequestCancel(ctx context.Context, id string, now time.Time) (shared.Task, error) {
	task, err := r.Get(ctx, id)
	if err != nil || !shared.Active(task.State) {
		return task, err
	}
	task.State = "cancel_requested"
	task.UpdatedAt = now
	_, err = r.db.ExecContext(ctx, `UPDATE tasks SET state='cancel_requested',updated_at=? WHERE id=?`, now.Format(time.RFC3339Nano), id)
	return task, err
}
func (r *Repository) Result(ctx context.Context, id string) (state, result string, err error) {
	err = r.db.QueryRowContext(ctx, `SELECT state,result FROM tasks WHERE id=?`, id).Scan(&state, &result)
	return
}
