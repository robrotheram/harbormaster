package store

import (
	"agent-workbench/internal/shared"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// createSampleTasks shares the project creation transaction. These are real,
// editable planning records, with no worker, executable instruction or attempt.
// Retries return the existing project and never overwrite edited sample tasks.
func (s *Store) createSampleTasks(ctx context.Context, tx *sql.Tx, project shared.Project) error {
	tasks := []struct{ title, description, priority string }{
		{"Build a welcome page", "Create a welcoming home page for a small task-tracking app. Include a project summary, a list of upcoming tasks, and a clear action to add a task.\n\nAcceptance criteria:\n- The page has helpful loading and empty states.\n- The layout works on a laptop and a phone.", "high"},
		{"Add task search and filters", "Add search by task title and filters for owner and status to the task-tracking app.\n\nAcceptance criteria:\n- Search and filters work together.\n- A clear-filters action restores the full list.\n- No-results text explains how to recover.", "normal"},
		{"Check keyboard navigation", "Test the welcome page and task filters using only a keyboard. Record any issues and propose fixes.\n\nAcceptance criteria:\n- Every control is reachable and has a visible focus indicator.\n- Inputs and buttons have accessible names.\n- Findings include steps to reproduce.", "normal"},
	}
	previous := ""
	for i, task := range tasks {
		id := shared.NewID("work_")
		work := shared.WorkItem{ID: id, ProjectID: project.ID, Kind: "software", State: "planned", CreatedAt: project.CreatedAt, UpdatedAt: project.CreatedAt}
		metadata, err := json.Marshal(work)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, s.query(`INSERT INTO work_items(id,project_id,create_key,metadata) VALUES(?,?,?,?)`), id, project.ID, fmt.Sprintf("sample-v1-%d", i+1), string(metadata)); err != nil {
			return err
		}
		description := task.description
		board := BoardItem{ID: id, ProjectID: project.ID, Stage: "planned", ExecutionState: "planned", Title: task.title, Description: &description, Priority: task.priority}
		if previous != "" {
			board.Dependencies = []string{previous}
		}
		if err = s.saveBoardItem(ctx, tx, board, 0); err != nil {
			return err
		}
		previous = id
	}
	return nil
}
