package workspace

import (
	"agent-workbench/internal/workspace/nativeagent"
	"context"
)

func (m *Manager) runNativeTask(ctx context.Context, provider, instruction, taskID string) (result string, code string) {
	m.event(provider, "agent.started", taskID, "", "")
	defer func() {
		kind := "agent.completed"
		if code != "" {
			kind = "agent.failed"
		}
		m.event(provider, kind, taskID, "", "")
	}()
	cfg := nativeagent.Config{TaskID: taskID, ProjectID: m.cfg.ProjectID, WorkspaceID: m.cfg.ID, DataDir: m.cfg.DataDir, HomeDir: m.homePath(), AgentKind: m.cfg.AgentKind, Token: m.cfg.Token, InferenceURL: m.cfg.InferenceURL, CodexModel: m.cfg.CodexModel, ClaudeModel: m.cfg.ClaudeModel, HermesModel: m.cfg.HermesModel, EgressURL: m.cfg.EgressURL, ConnectionMode: m.cfg.ConnectionMode}
	return nativeagent.Run(ctx, cfg, provider, instruction, &nativeEvents{m: m, source: provider, task: taskID}, &nativeEvents{m: m, source: provider, task: taskID})
}

type nativeOutput = nativeagent.Output
