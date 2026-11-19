package nativeagent

import (
	"agent-workbench/internal/workspace/toolconfig"
	"encoding/json"
	"path/filepath"
	"strings"
)

func prepareClaude(cfg Config, home string, env []string) (invocation, string) {
	var args []string

	if cfg.ClaudeModel == "" {
		return invocation{}, "PROVIDER_NOT_CONFIGURED"
	}
	env = append(env, "CLAUDE_CONFIG_DIR="+filepath.Join(home, ".claude"), "CLAUDE_CODE_OAUTH_TOKEN="+cfg.Token, "ANTHROPIC_BASE_URL="+strings.TrimRight(cfg.InferenceURL, "/")+"/claude", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1", "CLAUDE_CODE_DISABLE_BACKGROUND_TASKS=1")
	args = []string{"-p", "--output-format", "json", "--model", cfg.ClaudeModel, "--permission-mode", "bypassPermissions", "--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`, "--setting-sources", "", "--settings", `{"disableAllHooks":true}`}
	servers := map[string]any{}
	for _, entry := range toolconfig.Bundled() {
		servers[entry.Name] = map[string]any{"command": toolconfig.Command, "args": []string{"--mode", entry.Mode}}
	}
	raw, _ := json.Marshal(map[string]any{"mcpServers": servers})
	for i := range args {
		if args[i] == "--mcp-config" {
			args[i+1] = string(raw)
		}
	}

	if cfg.ReasoningEffort != "" {
		args = append(args, "--effort", cfg.ReasoningEffort)
	}

	return invocation{executable: "claude", args: args, env: env, resultFile: ""}, ""
}
