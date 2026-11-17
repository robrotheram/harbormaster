package store

import (
	"agent-workbench/internal/shared"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

// CreatePlannedTask saves editable planning content without allocating compute.
// The API claims the request fingerprint before this transaction; retries retain edits.
func (s *Store) CreatePlannedTask(ctx context.Context, in shared.BoardCreate, actor string, now time.Time) (shared.WorkItem, bool, error) {
	var item shared.WorkItem
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return item, false, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, s.query(`UPDATE projects SET metadata=metadata WHERE id=?`), in.ProjectID); err != nil {
		return item, false, err
	}
	var raw string
	err = tx.QueryRowContext(ctx, s.query(`SELECT metadata FROM work_items WHERE project_id=? AND create_key=?`), in.ProjectID, in.IdempotencyKey).Scan(&raw)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &item)
		return item, false, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return item, false, err
	}
	hash := sha256.Sum256([]byte(in.ProjectID + "\x00" + in.IdempotencyKey))
	item = shared.WorkItem{ID: "work_" + hex.EncodeToString(hash[:16]), ProjectID: in.ProjectID, Kind: in.Kind, ControllerID: actor, State: "planned", CreatedAt: now.UTC(), UpdatedAt: now.UTC()}
	data, _ := json.Marshal(item)
	if _, err = tx.ExecContext(ctx, s.query(`INSERT INTO work_items(id,project_id,create_key,metadata) VALUES(?,?,?,?)`), item.ID, item.ProjectID, in.IdempotencyKey, string(data)); err != nil {
		return item, false, err
	}
	board := BoardItem{ID: item.ID, ProjectID: item.ProjectID, Stage: "planned", ExecutionState: "planned", Title: in.Title, Description: &in.Description, Priority: "normal"}
	if err = s.saveBoardItem(ctx, tx, board, 0); err != nil {
		return item, false, err
	}
	return item, true, tx.Commit()
}
