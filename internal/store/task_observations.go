package store

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"agent-workbench/internal/shared"
)

// SaveObservedTasks writes changes from a single workspace snapshot. Stored
// task IDs remain bound to their original workspace, including during races
// with submissions. A conflicting ID does not discard other observations.
func (s *Store) SaveObservedTasks(ctx context.Context, workspace string, tasks []shared.Task) error {
	if len(tasks) == 0 {
		return nil
	}
	ids := make([]string, 0, len(tasks))
	metadata := make(map[string]string, len(tasks))
	for _, task := range tasks {
		if task.WorkspaceID != workspace {
			return ErrOwnership
		}
		body, err := json.Marshal(task)
		if err != nil {
			return err
		}
		if _, seen := metadata[task.ID]; !seen {
			ids = append(ids, task.ID)
		}
		metadata[task.ID] = string(body)
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	conflict := false
	for chunk := range slices.Chunk(ids, 250) {
		var values, placeholders []string
		args := []any{workspace}
		owners := []any{workspace}
		for _, id := range chunk {
			placeholders = append(placeholders, fmt.Sprintf("$%d", len(owners)+1))
			values = append(values, fmt.Sprintf("($%d,$%d)", len(args)+1, len(args)+2))
			args = append(args, id, metadata[id])
			owners = append(owners, id)
		}
		// Filter unchanged rows before the INSERT to avoid taking write locks
		// for every task in each poll. The conflict predicate closes the race
		// between reading the existing rows and writing the batch.
		query := `WITH incoming(id, metadata) AS (VALUES ` + strings.Join(values, ",") + `)
			INSERT INTO task_metadata(id,workspace_id,metadata)
			SELECT incoming.id,$1,incoming.metadata FROM incoming
			LEFT JOIN task_metadata AS stored ON stored.id=incoming.id
			WHERE stored.id IS NULL OR (stored.workspace_id=$1 AND stored.metadata<>incoming.metadata)
			ON CONFLICT(id) DO UPDATE SET metadata=excluded.metadata
			WHERE task_metadata.workspace_id=excluded.workspace_id AND task_metadata.metadata<>excluded.metadata`
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return err
		}
		var mismatch bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM task_metadata
			WHERE workspace_id<>$1 AND id IN (`+strings.Join(placeholders, ",")+`))`, owners...).Scan(&mismatch); err != nil {
			return err
		}
		conflict = conflict || mismatch
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if conflict {
		return ErrOwnership
	}
	return nil
}
