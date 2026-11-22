package workspace

import (
	"agent-workbench/internal/shared"
	"agent-workbench/internal/workspace/nativeagent"
	"agent-workbench/internal/workspace/setup"
	"context"
	"time"
)

const workspacePreviewGuidance = `Workspace delivery

If this task delivers or changes a runnable web app, leave a working Web preview for review before finishing. Do this even when the request does not explicitly ask for a preview. Use the workspace terminal to check workbench-preview status and reuse an existing preview if it serves the current app. Start or replace a preview only when needed for this app.

Use workbench-preview start --directory <workspace-relative-directory> --port <port> --command '<server-command>' to run the app under the workspace supervisor. Choose the directory and command from the project. Pass the assigned port using $PORT where supported. For a static site with an index.html, serve the directory that contains it, for example:
workbench-preview start --directory app --port 4173 --command 'python3 -m http.server "$PORT" --bind 127.0.0.1'

Before handing off, check that workbench-preview status reports the service ready and request the app's entry page from inside the workspace. Check that the response is the intended app. If startup fails, investigate and retry where possible. Keep the supervised service running for review. Ordinary background shell processes can be cleaned up when the agent run ends.

Say where to review the app in Workspace → Preview → Web. If the preview could not be verified, report the blocker. Use previews for runnable web deliverables and explicit preview requests. For other work, deliver the requested files, results, or explanation in the thread.`

func (m *Manager) runNativeTask(ctx context.Context, provider, instruction, taskID, model, effort string) (result string, code string) {
	m.event(provider, "agent.started", taskID, "", "")
	defer func() {
		kind := "agent.completed"
		if code != "" {
			kind = "agent.failed"
		}
		m.event(provider, kind, taskID, "", shared.SanitizedTaskError(code))
	}()
	cfg := nativeagent.Config{HermesMCPTransport: m.cfg.HermesMCPTransport, TaskID: taskID, ReasoningEffort: effort, ToolPath: setup.Path(m.homePath()), ProjectID: m.cfg.ProjectID, WorkspaceID: m.cfg.ID, DataDir: m.cfg.DataDir, HomeDir: m.homePath(), AgentKind: m.cfg.AgentKind, Token: m.cfg.Token, InferenceURL: m.cfg.InferenceURL, CodexModel: m.cfg.CodexModel, ClaudeModel: m.cfg.ClaudeModel, HermesModel: m.cfg.HermesModel, EgressURL: m.cfg.EgressURL, ConnectionMode: m.cfg.ConnectionMode}
	if model != "" {
		if provider == "codex" {
			cfg.CodexModel = model
		}
		if provider == "claude" {
			cfg.ClaudeModel = model
		}
	}
	var desktops int
	m.db.QueryRow(`SELECT count(*) FROM workspace_applications a JOIN retained_services s ON s.id=a.service_id WHERE s.state='active' AND s.expires_at>? AND json_extract(a.configuration,'$.kind')='desktop'`, time.Now().Unix()).Scan(&desktops)
	if desktops > 0 {
		cfg.Display = ":99"
	}
	instruction = workspacePreviewGuidance + "\n\n" + instruction
	return nativeagent.Run(ctx, cfg, provider, instruction, &nativeEvents{m: m, source: provider, task: taskID}, &nativeEvents{m: m, source: provider, task: taskID})
}
