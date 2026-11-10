package nativeagent

import (
	"os"
	"path/filepath"
)

func prepareHermes(cfg Config, home string, env []string) (invocation, string) {
	var args []string

	if cfg.AgentKind != "hermes" || cfg.HermesModel == "" {
		return invocation{}, "PROVIDER_NOT_CONFIGURED"
	}
	env = append(env, "HERMES_HOME="+home, "HERMES_MODEL="+cfg.HermesModel, "INFERENCE_URL="+cfg.InferenceURL, "WORKBENCH_INFERENCE_GRANT="+cfg.Token, "TERMINAL_ENV=local", "PYTHONPATH=/opt/hermes", "PYTHONUNBUFFERED=1")

	if project := cfg.ProjectID; project != "" {
		credential := filepath.Join(home, "workbench-mcp-token")
		if os.WriteFile(credential, []byte(cfg.Token), 0600) != nil {
			return invocation{}, "MCP_CREDENTIAL_UNAVAILABLE"
		}
		env = append(env, "WORKBENCH_MCP_URL=http://127.0.0.1:18085/mcp", "WORKBENCH_MCP_TOKEN_FILE="+credential, "WORKBENCH_PROJECT_ID="+project)
	}
	return invocation{executable: "/usr/local/bin/hermes-runner", args: args, env: env, resultFile: ""}, ""
}
