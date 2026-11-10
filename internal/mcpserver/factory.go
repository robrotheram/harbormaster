package mcpserver

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/url"
)

type FactoryInput struct {
	WorkspaceID         string `json:"workspace_id"`
	ID                  string `json:"id,omitempty"`
	Template            string `json:"template,omitempty"`
	IdempotencyKey      string `json:"idempotency_key,omitempty"`
	RetainedWorkspaceID string `json:"retained_workspace_id,omitempty"`
	ArtifactID          string `json:"artifact_id,omitempty"`
	TTL                 int    `json:"ttl_seconds,omitempty"`
}
type FactoryResult struct {
	Data map[string]any `json:"data"`
}

func registerFactoryTools(s *mcp.Server, c *controlClient) {
	for _, op := range []struct{ name, method, resource, suffix string }{
		{"factory_job_start", "POST", "recipe-jobs", ""}, {"factory_job_list", "GET", "recipe-jobs", ""},
		{"factory_job_status", "GET", "recipe-jobs", "id"}, {"factory_job_cancel", "POST", "recipe-jobs", "cancel"},
		{"factory_evidence_list", "GET", "artifacts", ""},
		{"factory_evidence_read", "GET", "artifacts", "content"},
		{"factory_preview_retain", "POST", "retained-previews", ""},
		{"factory_preview_status", "GET", "services", "id"}, {"factory_preview_revoke", "POST", "services", "revoke"}, {"factory_preview_release", "POST", "services", "release"},
	} {
		add(s, op.name, "Project-authorized factory operation. Reuse the receipt key on retries; previews require a separately registered workspace.", func(ctx context.Context, in FactoryInput) (out FactoryResult, err error) {
			base, err := workspacePath(in.WorkspaceID)
			if err != nil {
				return out, err
			}
			endpoint := base + "/" + op.resource
			if op.suffix != "" {
				if !identifier.MatchString(in.ID) {
					return out, invalid
				}
				endpoint += "/" + url.PathEscape(in.ID)
				if op.suffix != "id" {
					endpoint += "/" + op.suffix
				}
			}
			var body any
			if op.name == "factory_job_start" {
				body = map[string]any{"template": in.Template, "idempotency_key": in.IdempotencyKey}
			}
			if op.name == "factory_preview_retain" {
				if !keyPattern.MatchString(in.IdempotencyKey) {
					return out, invalid
				}
				body = map[string]any{"retained_workspace_id": in.RetainedWorkspaceID, "artifact_id": in.ArtifactID, "ttl_seconds": in.TTL, "idempotency_key": in.IdempotencyKey}
			}
			if op.method == "POST" && body == nil {
				body = map[string]any{}
			}
			err = c.do(ctx, op.method, endpoint, body, &out.Data)
			return
		})
	}
}
