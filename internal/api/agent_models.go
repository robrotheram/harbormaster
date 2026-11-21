package api

import (
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"net/http"
	"slices"
)

func (s *Server) registerAgentModels(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/workspaces/{id}/models", func(w http.ResponseWriter, r *http.Request) {
		ws, ok := s.workspace(w, r)
		if !ok {
			return
		}
		if err := s.refresh(r.Context(), &ws); err != nil {
			apiError(w, err)
			return
		}
		if !ws.Ready {
			shared.Error(w, 409, "WORKSPACE_STOPPED")
			return
		}
		status, body, err := s.Runtime.Agent(r.Context(), ws.ID, "GET", "/v1/models", nil)
		var catalog shared.AgentModels
		if err != nil || status != 200 || json.Unmarshal(body, &catalog) != nil || catalog.WorkspaceID != ws.ID {
			shared.Error(w, 503, "MODEL_CATALOG_UNAVAILABLE")
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		shared.JSON(w, 200, catalog)
	})
}

func validExplicitSelection(selection *shared.AgentModel) bool {
	return selection == nil || (selection.Model != "" && shared.ValidWorkSelection(selection.Provider, selection.Model) && shared.ValidReasoningEffort(selection.Provider, selection.ReasoningEffort))
}

func (s *Server) checkAgentSelection(ctx context.Context, workspace string, selection *shared.AgentModel) error {
	status, raw, err := s.Runtime.Agent(ctx, workspace, "GET", "/v1/models", nil)
	var catalog shared.AgentModels
	if err != nil || status != 200 || json.Unmarshal(raw, &catalog) != nil || catalog.WorkspaceID != workspace {
		return shared.CodedError{Status: 503, Code: "MODEL_CATALOG_UNAVAILABLE"}
	}
	if !slices.ContainsFunc(catalog.Items, func(model shared.AgentModel) bool {
		return model.Provider == selection.Provider && model.Model == selection.Model
	}) {
		return shared.CodedError{Status: 400, Code: "MODEL_NOT_AVAILABLE"}
	}
	return nil
}
