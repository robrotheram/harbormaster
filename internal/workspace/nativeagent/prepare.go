package nativeagent

import (
	"agent-workbench/internal/egress"
	"os"
	"path/filepath"
)

func prepare(cfg Config, provider string) (invocation, string) {
	if provider != "codex" && provider != "claude" && provider != "hermes" {
		return invocation{}, "PROVIDER_NOT_CONFIGURED"
	}
	home := filepath.Join(cfg.HomeDir, "agents", provider)
	if os.MkdirAll(home, 0700) != nil {
		return invocation{}, "STORAGE_UNAVAILABLE"
	}
	// Only a generation-bound workspace grant, never provider or Vault credentials.
	env := []string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=" + home, "LANG=C.UTF-8", "TMPDIR=/tmp", "DATA_DIR=" + cfg.DataDir, "DISABLE_AUTOUPDATER=1", "DISABLE_UPDATES=1", "USE_BUILTIN_RIPGREP=0"}

	if cfg.TaskID != "" {
		env = append(env, "WORKBENCH_TASK_ID="+cfg.TaskID)
	}
	if cfg.AgentKind == "hermes" {
		env = append(env, "WORKSPACE_TOKEN="+cfg.Token, "PROJECT_ID="+cfg.ProjectID, "WORKSPACE_ID="+cfg.WorkspaceID)
	}
	if cfg.EgressURL != "" && cfg.ConnectionMode != "open" {
		env = egress.Environment(env)
	}
	switch provider {
	case "codex":
		return prepareCodex(cfg, home, env)
	case "claude":
		return prepareClaude(cfg, home, env)
	case "hermes":
		return prepareHermes(cfg, home, env)
	default:
		return invocation{}, "PROVIDER_NOT_CONFIGURED"
	}
}
