package mcpserver

import (
	"agent-workbench/internal/shared"
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/url"
)

type WorkList struct {
	Items []shared.WorkItem `json:"items"`
}
type WorkCreate struct {
	Instruction         *string  `json:"instruction,omitempty"`
	ArtifactPaths       []string `json:"artifact_paths,omitempty"`
	ProjectID           string   `json:"project_id"`
	WorkspaceID         string   `json:"workspace_id,omitempty"`
	SpecificationPath   string   `json:"specification_path,omitempty"`
	SpecificationDigest string   `json:"specification_digest,omitempty"`
	Kind                string   `json:"kind"`
	IdempotencyKey      string   `json:"idempotency_key"`
}

func registerWorkTools(s *mcp.Server, c *controlClient) {
	registerWorkExecutionTools(s, c)
	registerBoardTools(s, c)
	add(s, "work_list", "List up to 200 work records within the authorized project.", func(ctx context.Context, in ProjectFilter) (out WorkList, err error) {
		err = c.do(ctx, "GET", "/v1/work-items?project_id="+url.QueryEscape(in.ProjectID), nil, &out)
		return
	})
	add(s, "work_create", "Save a draft with an inline instruction (up to 64 KiB) or a private specification path and SHA-256 digest. Omit workspace_id to provision an approved private Codex worker for inline input. Running requires work_run.", func(ctx context.Context, in WorkCreate) (out shared.WorkItem, err error) {
		err = c.do(ctx, "POST", "/v1/work-items", in, &out)
		return
	})
}

type WorkReference struct {
	ProjectID string `json:"project_id"`
	ID        string `json:"id"`
}
type WorkRun struct {
	ProjectID      string `json:"project_id"`
	ID             string `json:"id"`
	IdempotencyKey string `json:"idempotency_key"`
}
type WorkReview struct {
	ProjectID         string `json:"project_id"`
	ID                string `json:"id"`
	Accepted          bool   `json:"accepted"`
	ExpectedAttemptID string `json:"expected_attempt_id"`
	Feedback          string `json:"feedback,omitempty"`
}

func registerWorkExecutionTools(s *mcp.Server, c *controlClient) {
	add(s, "work_run", "Queue an attempt; reuse the delivery key on retries.", func(ctx context.Context, in WorkRun) (out shared.WorkAttempt, err error) {
		err = c.do(ctx, "POST", "/v1/work-items/"+url.PathEscape(in.ID)+"/run", map[string]string{"project_id": in.ProjectID, "idempotency_key": in.IdempotencyKey}, &out)
		return
	})
	add(s, "work_attempts", "List project-owned execution attempts.", func(ctx context.Context, in WorkReference) (out struct {
		Items []shared.WorkAttempt `json:"items"`
	}, err error) {
		err = c.do(ctx, "GET", "/v1/work-items/"+url.PathEscape(in.ID)+"/attempts?project_id="+url.QueryEscape(in.ProjectID), nil, &out)
		return
	})
	add(s, "work_cancel", "Cancel work and its active native task.", func(ctx context.Context, in WorkReference) (out shared.WorkItem, err error) {
		err = c.do(ctx, "POST", "/v1/work-items/"+url.PathEscape(in.ID)+"/cancel", map[string]string{"project_id": in.ProjectID}, &out)
		return
	})
	add(s, "work_review", "Accept a revision or request changes with feedback.", func(ctx context.Context, in WorkReview) (out shared.WorkItem, err error) {
		err = c.do(ctx, "POST", "/v1/work-items/"+url.PathEscape(in.ID)+"/review", map[string]any{"project_id": in.ProjectID, "accepted": in.Accepted, "expected_attempt_id": in.ExpectedAttemptID, "feedback": in.Feedback}, &out)
		return
	})
}
