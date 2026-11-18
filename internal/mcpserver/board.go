package mcpserver

import (
	"agent-workbench/internal/shared"
	"agent-workbench/internal/store"
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/url"
)

type BoardCard struct {
	store.BoardItem
	Work      shared.WorkItem `json:"work"`
	Dirty     bool            `json:"dirty"`
	Attention bool            `json:"attention"`
}
type BoardUpdate struct {
	ProjectID   string  `json:"project_id"`
	ID          string  `json:"id"`
	Revision    int     `json:"revision"`
	Stage       string  `json:"stage"`
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
}

func registerBoardTools(s *mcp.Server, c *controlClient) {
	add(s, "board_list", "List this project's issues, including archived issues, current stages and revision numbers. Includes local issues and linked repository issues.", func(ctx context.Context, in ProjectFilter) (out struct {
		Items []BoardCard `json:"items"`
	}, err error) {
		err = c.do(ctx, "GET", "/v1/board?project_id="+url.QueryEscape(in.ProjectID), nil, &out)
		return
	})
	add(s, "board_create", "Create a standalone issue in Planned without a GitHub/GitLab connection. Supply title, optional description, kind (software, research or digest), and a stable idempotency_key. Saves a private work draft; does not run it. Use work_run only when asked to execute.", func(ctx context.Context, in shared.BoardCreate) (out BoardCard, err error) {
		err = c.do(ctx, "POST", "/v1/board", in, &out)
		return
	})
	add(s, "board_update", "Edit a ticket title or Markdown description, or move an issue using its current revision from board_list. Stages: planned, running, review, done, archived. Archive hides an issue; restore by setting its archived_from stage. Moving finished work to planned, running or review explicitly reopens the task, preserving its prior decision. Moving never starts the agent or approves a review. On BOARD_REVISION_CONFLICT, reread before applying the user's intended change. Does not sync changes to a repository.", func(ctx context.Context, in BoardUpdate) (out BoardCard, err error) {
		if !shared.ValidID(in.ID) {
			return out, invalid
		}
		err = c.do(ctx, "POST", "/v1/board/"+in.ID, map[string]any{"project_id": in.ProjectID, "revision": in.Revision, "stage": in.Stage, "title": in.Title, "description": in.Description}, &out)
		return
	})
}
