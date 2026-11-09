package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfileMCPConfigRemovesDeselectedToolsPreservingProvider(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	original := `model = "custom-model"
[model_providers.workbench]
base_url = "http://127.0.0.1:18081/codex"
[mcp_servers.workspace-files]
command = "old"
[mcp_servers.project-git]
command = "old"
[other_settings]
keep = true
`
	os.WriteFile(path, []byte(original), 0600)
	m := &Manager{cfg: Config{MCPs: []string{"project-git"}}}
	if err := m.syncMCPConfig(path); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	s := string(b)
	if strings.Contains(s, "workspace-files") || !strings.Contains(s, "[mcp_servers.project-git]") || !strings.Contains(s, `model = "custom-model"`) || !strings.Contains(s, "keep = true") {
		t.Fatal(s)
	}
	m.cfg.MCPs = []string{}
	if err := m.syncMCPConfig(path); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(path)
	if strings.Contains(string(b), "[mcp_servers.") {
		t.Fatal("empty profile retained MCP servers")
	}
	m.cfg.MCPs = nil
	if len(m.mcpEntries()) != 2 {
		t.Fatal("legacy default changed")
	}
}
