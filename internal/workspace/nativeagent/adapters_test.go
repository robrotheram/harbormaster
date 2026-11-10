package nativeagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdapterPlansUseOnlyExplicitWorkspaceAuthority(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "host-secret-must-not-cross")
	t.Setenv("ANTHROPIC_API_KEY", "host-secret-must-not-cross")
	root := t.TempDir()
	cfg := Config{DataDir: root, HomeDir: root, AgentKind: "hermes", Token: "workspace-generation-grant", InferenceURL: "http://inference.internal", CodexModel: "approved-codex", ClaudeModel: "approved-claude", HermesModel: "approved-hermes"}
	for _, provider := range []string{"codex", "claude", "hermes"} {
		plan, code := prepare(cfg, provider)
		if code != "" {
			t.Fatal(provider, code)
		}
		if plan.resultFile != "" {
			defer os.Remove(plan.resultFile)
		}
		joined := strings.Join(plan.env, "\n")
		if strings.Contains(joined, "host-secret") || !strings.Contains(joined, cfg.Token) {
			t.Fatal("adapter inherited host authority", provider)
		}
		if provider != "hermes" {
			args := strings.Join(plan.args, " ")
			if !strings.Contains(args, "workspace-files") || !strings.Contains(args, "project-git") {
				t.Fatal("adapter omitted bundled file/Git tools", provider)
			}
		}
	}
	if _, code := prepare(cfg, "../../escape"); code != "PROVIDER_NOT_CONFIGURED" {
		t.Fatal("unrecognized provider accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "escape")); !os.IsNotExist(err) {
		t.Fatal("invalid provider prepared persistent state")
	}
}

func TestHermesInvocationCarriesStableTaskOccurrence(t *testing.T) {
	root := t.TempDir()
	config := Config{DataDir: root, HomeDir: root, AgentKind: "hermes", Token: "workspace-generation-grant", ProjectID: "personal", WorkspaceID: "workspace_one", TaskID: "task_occurrence_one", HermesModel: "approved-model", InferenceURL: "http://inference.internal"}
	plan, code := prepare(config, "hermes")
	if code != "" {
		t.Fatal(code)
	}
	env := strings.Join(plan.env, "\n")
	for _, expected := range []string{"WORKBENCH_TASK_ID=task_occurrence_one", "PROJECT_ID=personal", "WORKSPACE_ID=workspace_one", "WORKBENCH_MCP_URL=http://127.0.0.1:18085/mcp"} {
		if !strings.Contains(env, expected) {
			t.Fatalf("missing occurrence/binding: %s", expected)
		}
	}
}
