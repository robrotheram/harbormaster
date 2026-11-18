package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"agent-workbench/internal/jsonmetadata"
	"agent-workbench/internal/shared"
)

var ErrBoardConflict = errors.New("board revision conflict")

// BoardItem stores user-visible planning metadata separately from execution.
// Mutable ticket descriptions are separate from immutable execution instructions.
type BoardItem struct {
	Number         int          `json:"number,omitempty"`
	OwnerID        string       `json:"owner_id,omitempty"`
	Owner          string       `json:"owner,omitempty"`
	Priority       string       `json:"priority,omitempty"`
	Dependencies   []string     `json:"dependencies,omitempty"`
	Description    *string      `json:"description,omitempty"`
	ArchivedFrom   string       `json:"archived_from,omitempty"`
	ID             string       `json:"id"`
	ProjectID      string       `json:"project_id"`
	Revision       int          `json:"revision"`
	Stage          string       `json:"stage"`
	ExecutionState string       `json:"execution_state"`
	Title          string       `json:"title,omitempty"`
	Source         *BoardSource `json:"source,omitempty"`
}
type BoardSource struct {
	Description  *string `json:"description,omitempty"`
	ConnectionID string  `json:"connection_id"`
	Repository   string  `json:"repository"`
	Number       int     `json:"number"`
	URL          string  `json:"url"`
	Title        string  `json:"title"`
	State        string  `json:"state"`
	SyncedAt     string  `json:"synced_at"`
	Error        string  `json:"error,omitempty"`
}

func BoardStage(state string) string {
	switch state {
	case "in_progress", "running", "submitting", "dispatching", "blocked", "cancel_requested":
		return "running"
	case "awaiting_review":
		return "review"
	case "completed", "accepted", "closed":
		return "done"
	}
	return "planned"
}
func ValidBoardStage(stage string) bool {
	return stage == "planned" || stage == "running" || stage == "review" || stage == "done" || stage == "archived"
}
func (b BoardItem) Effective(work shared.WorkItem) BoardItem {
	b.ID = work.ID
	b.ProjectID = work.ProjectID
	if b.Stage == "" || (b.Stage != "archived" && b.ExecutionState != work.State) {
		b.Stage = BoardStage(work.State)
	}
	// A review outcome is authoritative. Planning metadata cannot reopen
	// accepted or closed work, including metadata saved before this rule.
	if BoardStage(work.State) == "done" {
		if b.Stage == "archived" {
			b.ArchivedFrom = "done"
		} else {
			b.Stage = "done"
		}
	}
	b.ExecutionState = work.State
	return b
}
func (b BoardItem) RemoteState() string {
	if b.Stage == "archived" {
		if b.ArchivedFrom == "done" {
			return "closed"
		}
		return "open"
	}
	if b.Stage == "done" {
		return "closed"
	}
	return "open"
}
func (b BoardItem) Dirty() bool {
	return b.Source != nil && (b.Title != b.Source.Title || b.RemoteState() != b.Source.State || (b.Description != nil && (b.Source.Description == nil || *b.Description != *b.Source.Description)))
}
func (s *Store) initBoard(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS board_items(id TEXT PRIMARY KEY,project_id TEXT NOT NULL,revision INTEGER NOT NULL,metadata TEXT NOT NULL)`)
	return err
}
func (s *Store) BoardItem(ctx context.Context, project, id string) (BoardItem, error) {
	item, err := jsonmetadata.Get[BoardItem](ctx, s.DB, `SELECT metadata FROM board_items WHERE project_id=$1 AND id=$2`, project, id)
	if errors.Is(err, sql.ErrNoRows) {
		number, e := s.workReference(ctx, project, id)
		return BoardItem{ID: id, ProjectID: project, Number: number}, e
	}
	if err == nil {
		item.Number, err = s.workReference(ctx, project, id)
	}
	return item, err
}

type boardWriter interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func (s *Store) SaveBoardItem(ctx context.Context, item BoardItem, expected int) error {
	return s.saveBoardItem(ctx, s.DB, item, expected)
}
func (s *Store) saveBoardItem(ctx context.Context, db boardWriter, item BoardItem, expected int) error {
	item.Revision = expected + 1
	b, err := json.Marshal(item)
	if err != nil {
		return err
	}
	if expected == 0 {
		res, err := db.ExecContext(ctx, `INSERT INTO board_items(id,project_id,revision,metadata) VALUES($1,$2,$3,$4) ON CONFLICT(id) DO NOTHING`, item.ID, item.ProjectID, item.Revision, string(b))
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return ErrBoardConflict
		}
		return nil
	}
	res, err := db.ExecContext(ctx, `UPDATE board_items SET revision=$1,metadata=$2 WHERE id=$3 AND project_id=$4 AND revision=$5`, item.Revision, string(b), item.ID, item.ProjectID, expected)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return ErrBoardConflict
	}
	return nil
}
