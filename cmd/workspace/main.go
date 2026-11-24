package main

import (
	"agent-workbench/internal/shared"
	"agent-workbench/internal/workspace"
	"agent-workbench/internal/workspace/nativeagent"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	if code, handled := workspace.CommandSupervisor(os.Args[1:]); handled {
		os.Exit(code)
	}
	token, e := shared.Secret("WORKSPACE_TOKEN")
	if e != nil || token == "" {
		slog.Error("workspace token required")
		os.Exit(1)
	}
	var matrix *shared.HermesMatrix
	if raw := os.Getenv("HERMES_MATRIX_CONFIG"); raw != "" {
		if json.Unmarshal([]byte(raw), &matrix) != nil {
			slog.Error("invalid Matrix configuration")
			os.Exit(1)
		}
	}
	m, e := workspace.New(workspace.Config{HermesMCPTransport: nativeagent.MCPTransport(shared.Env("HERMES_MCP_TRANSPORT", "")), HermesMatrix: matrix, HermesModel: shared.Env("HERMES_MODEL", ""), ConnectionMode: shared.Env("WORKSPACE_CONNECTION_MODE", ""), EgressURL: shared.Env("EGRESS_URL", ""), ProjectID: shared.Env("PROJECT_ID", "default"), HomeID: shared.Env("AGENT_HOME_ID", ""), AgentKind: shared.Env("AGENT_KIND", ""), Name: shared.Env("WORKSPACE_NAME", ""), Console: shared.Env("WORKSPACE_T3", "false") == "true", Branch: shared.Env("WORKSPACE_BRANCH", ""), DefaultProvider: shared.Env("WORKSPACE_AGENT", ""), CodexModel: shared.Env("CODEX_MODEL", ""), ClaudeModel: shared.Env("CLAUDE_MODEL", ""), ID: shared.Env("WORKSPACE_ID", "local"), DataDir: shared.Env("DATA_DIR", "/data"), Token: token, InferenceURL: shared.Env("INFERENCE_URL", "http://inference-gateway:8080"), BrokerURL: shared.Env("BROKER_URL", "http://credential-broker:8080"), Model: shared.Env("INFERENCE_MODEL", "fixture"), MonitorProcesses: shared.Env("MONITOR_PROCESSES", "true") == "true"})
	if e != nil {
		if errors.Is(e, workspace.ErrPreReleaseSchema) {
			slog.Error(workspace.ErrPreReleaseSchema.Error())
		} else {
			slog.Error("workspace storage initialization failed")
		}
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	e = shared.ServeContext(ctx, shared.Env("LISTEN_ADDR", ":8080"), m.Handler())
	closeErr := m.Close()
	if e != nil {
		slog.Error("workspace server stopped")
		os.Exit(1)
	}
	if closeErr != nil {
		slog.Error("workspace supervisor cleanup failed")
		os.Exit(1)
	}
}
