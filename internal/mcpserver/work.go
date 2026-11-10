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
	ProjectID           string `json:"project_id"`
	WorkspaceID         string `json:"workspace_id"`
	SpecificationPath   string `json:"specification_path"`
	SpecificationDigest string `json:"specification_digest"`
	Kind                string `json:"kind"`
	IdempotencyKey      string `json:"idempotency_key"`
}

func registerWorkTools(s *mcp.Server, c *controlClient) {
	add(s, "work_list", "List up to 200 draft/cancelled work records within the authorized project.", func(ctx context.Context, in ProjectFilter) (out WorkList, err error) {
		err = c.do(ctx, "GET", "/v1/work-items?project_id="+url.QueryEscape(in.ProjectID), nil, &out)
		return
	})
	add(s, "work_create", "Save a draft referencing a private workspace specification and SHA-256 digest. Does not verify or execute that specification.", func(ctx context.Context, in WorkCreate) (out shared.WorkItem, err error) {
		err = c.do(ctx, "POST", "/v1/work-items", in, &out)
		return
	})
}
