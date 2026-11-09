package workspacesvc

import (
	"agent-workbench/internal/lifecycle"
	"agent-workbench/internal/shared"
	"context"
	"errors"
)

// AssistantRepository adds creation to the workspace application boundary. The
// caller holds the project assistant lock and supplies creation admission; existing
// workspaces additionally use their normal lifecycle lock.
type AssistantRepository interface {
	Repository
	Workspace(context.Context, string) (shared.Workspace, error)
	Create(context.Context, shared.Workspace, string) error
}

// EnsureAssistant owns selection, configuration and startup of a project's
// private Hermes workspace. Browsers need not reproduce this workflow.
func (s *Service) EnsureAssistant(ctx context.Context, repo AssistantRepository, project, key string, memory int64, admission func() func()) (shared.Workspace, bool, error) {
	config, err := repo.HermesSetup(ctx, project)
	if err != nil || !s.HermesAvailable(ctx) {
		return shared.Workspace{}, false, errors.New("HERMES_SETUP_REQUIRED")
	}
	release := admission()
	held := true
	defer func() {
		if held {
			release()
		}
	}()
	all, err := repo.Workspaces(ctx)
	if err != nil {
		return shared.Workspace{}, false, err
	}
	var ws shared.Workspace
	for _, item := range all {
		if item.ProjectID == project && item.AgentKind == "hermes" {
			if ws.ID != "" {
				return shared.Workspace{}, false, errors.New("ASSISTANT_WORKSPACE_AMBIGUOUS")
			}
			ws = item
		}
	}
	created := false
	if ws.ID == "" {
		if len(all) >= 100 {
			return ws, false, errors.New("QUOTA_EXCEEDED")
		}
		now := s.Now()
		ws = shared.Workspace{ID: shared.NewID("ws_"), ProjectID: project, HomeID: shared.NewID("home_"), Name: "Hermes", AgentKind: "hermes", ProfileID: "general", State: "STOPPED", CreatedAt: now, UpdatedAt: now, RuntimeType: "docker", IsolationStatus: "unverified", NetworkStatus: "unverified", CPULimit: 1, MemoryLimitBytes: memory, ConnectionProfileID: config.ConnectionProfileID}
		if err = s.ResolveConnectionProfile(ctx, &ws); err != nil {
			return ws, false, err
		}
		if err = repo.Create(ctx, ws, "assistant:"+key); err != nil {
			return ws, false, err
		}
		created = true
	}
	release()
	held = false
	unlock := s.Coordinator.Lock(ws.ID)
	defer unlock()
	ws, err = repo.Workspace(ctx, ws.ID)
	if err != nil {
		return ws, created, err
	}
	if ws.ConnectionProfileID != config.ConnectionProfileID {
		if err = s.Refresh(ctx, &ws); err != nil {
			return ws, created, err
		}
		previous := ws
		ws.ConnectionProfileID = config.ConnectionProfileID
		if config.ConnectionProfileID == "" {
			ws.ConnectionProfileRevision = 0
			ws.Connections = &shared.ConnectionPolicy{Mode: "blocked", Destinations: []string{}}
			ws.GitHubAccess = false
		} else if err = s.ResolveConnectionProfile(ctx, &ws); err != nil {
			return ws, created, err
		}
		if err = lifecycle.CheckConnections(ws); err != nil {
			return ws, created, err
		}
		if ws.State != "STOPPED" {
			if err = s.ConfigureNetwork(ctx, ws); err != nil {
				return ws, created, err
			}
		}
		if err = repo.Save(ctx, ws); err != nil {
			if ws.State != "STOPPED" {
				s.report("assistant_policy_rollback", previous, s.ConfigureNetwork(ctx, previous))
			}
			return ws, created, err
		}
	}
	err = s.Start(ctx, &ws)
	return ws, created, err
}
