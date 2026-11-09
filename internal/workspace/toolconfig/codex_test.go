package toolconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExistingNativeHomeRestoresFixedToolsAndPreservesProviderSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	original := `model = "custom-model"
[model_providers.workbench]
base_url = "http://127.0.0.1:18081/codex"
[mcp_servers.project-git]
command = "old"
[mcp_servers.project-git.env]
OLD_OVERRIDE = "retired"
[other_settings]
keep = true
`
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SyncCodex(path); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	config := string(first)
	if !strings.Contains(config, `model = "custom-model"`) || !strings.Contains(config, "keep = true") || strings.Contains(config, "OLD_OVERRIDE") || strings.Contains(config, `command = "old"`) {
		t.Fatal("tool restoration altered native provider settings or retained obsolete tools", config)
	}
	for _, entry := range Bundled() {
		if strings.Count(config, "[mcp_servers."+entry.Name+"]") != 1 {
			t.Fatal("fixed file/Git bundle was not restored exactly once", config)
		}
	}
	if err := SyncCodex(path); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil || string(second) != config {
		t.Fatal("restarting altered an already synchronized configuration", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("native configuration lost private permissions", err)
	}
}
