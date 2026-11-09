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
	cfg := Config{DataDir: root, HomeDir: root, AgentKind: "hermes", Token: "workspace-generation-grant", InferenceURL: "http://inference.internal", CodexModel: "approved-codex", ClaudeModel: "approved-claude", HermesModel: "approved-hermes", MCPs: []MCP{{Name: "workspace-files", Mode: "files"}}}
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
			if !strings.Contains(args, "workspace-files") || strings.Contains(args, "project-git") {
				t.Fatal("adapter changed selected tool set", provider)
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
