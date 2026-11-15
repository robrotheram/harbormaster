package api

import (
	"agent-workbench/internal/shared"
	"context"
	"database/sql"
	"errors"
)

// workspaceInProject expresses execution membership, not metadata authorization.
// Shared mode is one operator trust domain; its runtime/grant owner stays default.
func (s *Server) workspaceInProject(ws shared.Workspace, project string) bool {
	if s.cfg.SharedWorkspaceID == "" {
		return ws.ProjectID == project
	}
	return shared.ValidID(project) && ws.ID == s.cfg.SharedWorkspaceID && ws.ProjectID == "default"
}
func (s *Server) sharedWorkspace(ws shared.Workspace) bool {
	return s.cfg.SharedWorkspaceID != "" && ws.ID == s.cfg.SharedWorkspaceID && ws.ProjectID == "default"
}

// EnsureSharedWorkspace provisions metadata only. Compute starts through the usual
// admission/lifecycle path. The fixed primary key prevents replica/retry duplicates.
func (s *Server) EnsureSharedWorkspace(ctx context.Context) (shared.Workspace, error) {
	id := s.cfg.SharedWorkspaceID
	if !shared.ValidID(id) {
		return shared.Workspace{}, errors.New("invalid shared workspace ID")
	}
	all, err := s.Store.Workspaces(ctx)
	if err != nil {
		return shared.Workspace{}, err
	}
	for _, other := range all {
		if other.ID != id && (other.State != "STOPPED" || other.Ready || other.ActiveRuns > 0 || other.BackgroundJobs > 0) {
			return shared.Workspace{}, errors.New("stop existing workers before enabling shared mode")
		}
	}
	ws, err := s.Store.Workspace(ctx, id)
	if err == nil {
		if ws.ProjectID != "default" || ws.AgentKind != "t3" || ws.ParentID != "" {
			return ws, errors.New("shared workspace identity conflict")
		}
		ws.Shared = true
		return ws, nil
	}
	if err != sql.ErrNoRows {
		return ws, err
	}
	if _, err = s.Store.Project(ctx, "default"); err != nil {
		return ws, err
	}
	now := s.now()
	ws = shared.Workspace{ID: id, ProjectID: "default", Shared: true, HomeID: shared.NewID("home_"), Name: "Shared workspace", AgentKind: "t3", TemplateID: "development-v1", State: "STOPPED", CreatedAt: now, UpdatedAt: now, RuntimeType: "kubernetes", IsolationStatus: "unverified", NetworkStatus: "unverified", CPULimit: 1, MemoryLimitBytes: s.cfg.WorkspaceMemory}
	if err = s.Store.Create(ctx, ws, "shared-workspace:"+id); err != nil {
		existing, readErr := s.Store.Workspace(ctx, id)
		if readErr == nil && existing.ProjectID == "default" && existing.AgentKind == "t3" && existing.ParentID == "" {
			existing.Shared = true
			return existing, nil
		}
		return shared.Workspace{}, err
	}
	return ws, nil
}
