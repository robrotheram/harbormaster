package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"agent-workbench/internal/jsonmetadata"
	"agent-workbench/internal/shared"
)

// CreatePlannedTask saves editable planning content without allocating compute.
// The API claims the request fingerprint before this transaction; retries retain edits.
func (s *Store) CreatePlannedTask(ctx context.Context, in shared.BoardCreate, actor string, source *BoardSource, now time.Time) (shared.WorkItem, bool, error) {
	var item shared.WorkItem
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return item, false, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE projects SET metadata=metadata WHERE id=$1`, in.ProjectID); err != nil {
		return item, false, err
	}
	item, err = jsonmetadata.Get[shared.WorkItem](ctx, tx, `SELECT metadata FROM work_items WHERE project_id=$1 AND create_key=$2`, in.ProjectID, in.IdempotencyKey)
	if !errors.Is(err, sql.ErrNoRows) {
		return item, false, err
	}
	hash := sha256.Sum256([]byte(in.ProjectID + "\x00" + in.IdempotencyKey))
	item = shared.WorkItem{ID: "work_" + hex.EncodeToString(hash[:16]), ProjectID: in.ProjectID, Kind: in.Kind, ControllerID: actor, State: "planned", CreatedAt: now.UTC(), UpdatedAt: now.UTC()}
	data, _ := json.Marshal(item)
	if _, err = tx.ExecContext(ctx, `INSERT INTO work_items(id,project_id,create_key,metadata) VALUES($1,$2,$3,$4)`, item.ID, item.ProjectID, in.IdempotencyKey, string(data)); err != nil {
		return item, false, err
	}
	board := BoardItem{ID: item.ID, ProjectID: item.ProjectID, Stage: "planned", ExecutionState: "planned", Title: in.Title, Description: &in.Description, Priority: "normal", Source: source}
	if source != nil && source.State == "closed" {
		board.Stage = "done"
	}
	if err = s.saveBoardItem(ctx, tx, board, 0); err != nil {
		return item, false, err
	}
	return item, true, tx.Commit()
}
