package workspace

import (
	"agent-workbench/internal/workspace/nativeagent"
	"context"
	"time"
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
	cfg := nativeagent.Config{HermesMCPTransport: m.cfg.HermesMCPTransport, TaskID: taskID, ProjectID: m.cfg.ProjectID, WorkspaceID: m.cfg.ID, DataDir: m.cfg.DataDir, HomeDir: m.homePath(), AgentKind: m.cfg.AgentKind, Token: m.cfg.Token, InferenceURL: m.cfg.InferenceURL, CodexModel: m.cfg.CodexModel, ClaudeModel: m.cfg.ClaudeModel, HermesModel: m.cfg.HermesModel, EgressURL: m.cfg.EgressURL, ConnectionMode: m.cfg.ConnectionMode}
	var desktops int
	m.db.QueryRow(`SELECT count(*) FROM workspace_applications a JOIN retained_services s ON s.id=a.service_id WHERE s.state='active' AND s.expires_at>? AND json_extract(a.configuration,'$.kind')='desktop'`, time.Now().Unix()).Scan(&desktops)
	if desktops > 0 {
		cfg.Display = ":99"
	}
	instruction = "Workspace application preview: when asked to run or preview an app, use the terminal command workbench-preview start --directory <workspace-relative-directory> --port <port> --command <server-command>. Use $PORT in the server command where supported. This starts a supervised process that survives completion of this agent run. Ordinary background shell processes may be cleaned up when your run ends. Check workbench-preview status before claiming that a preview is running; the workbench forwards the app to its Web preview. Do not tell the user to run a local command or open the container's localhost URL on their computer.\n\n" + instruction
	return nativeagent.Run(ctx, cfg, provider, instruction, &nativeEvents{m: m, source: provider, task: taskID}, &nativeEvents{m: m, source: provider, task: taskID})
}

type nativeOutput = nativeagent.Output
