package nativeagent

import (
	"agent-workbench/internal/workspace/toolconfig"
	"os"
	"path/filepath"
	"strings"
)

func prepareCodex(cfg Config, home string, env []string) (invocation, string) {
	var args []string
	var resultFile string

	if cfg.CodexModel == "" {
		return invocation{}, "PROVIDER_NOT_CONFIGURED"
	}
	codexHome := filepath.Join(home, ".codex")
	if os.MkdirAll(codexHome, 0700) != nil {
		return invocation{}, "STORAGE_UNAVAILABLE"
	}
	env = append(env, "CODEX_HOME="+codexHome, "WORKBENCH_INFERENCE_GRANT="+cfg.Token)
	file, err := os.CreateTemp(home, "result-*.txt")
	if err != nil {
		return invocation{}, "STORAGE_UNAVAILABLE"
	}
	resultFile = file.Name()
	file.Close()
	args = []string{"exec", "--ignore-user-config", "--ignore-rules", "--skip-git-repo-check", "--ephemeral", "--dangerously-bypass-approvals-and-sandbox", "--color", "never", "--json", "-m", cfg.CodexModel,
		"-c", `model_provider="workbench"`, "-c", `model_providers.workbench.name="Workspace subscription gateway"`,
		"-c", "model_providers.workbench.base_url=" + tomlString(strings.TrimRight(cfg.InferenceURL, "/")+"/codex"),
		"-c", `model_providers.workbench.wire_api="responses"`, "-c", `model_providers.workbench.env_key="WORKBENCH_INFERENCE_GRANT"`,
		"-c", `model_providers.workbench.supports_websockets=false`, "-o", resultFile, "-"}
	args = args[:len(args)-1]
	for _, entry := range toolconfig.Bundled() {
		args = append(args, "-c", "mcp_servers."+entry.Name+".command="+tomlString(toolconfig.Command), "-c", "mcp_servers."+entry.Name+".args=[\"--mode\","+tomlString(entry.Mode)+"]")
	}
	args = append(args, "-")

	return invocation{executable: "codex", args: args, env: env, resultFile: resultFile}, ""
}
