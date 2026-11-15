package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepairOnlyEmptyConsoleModel(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config.toml")
	settings := filepath.Join(dir, "settings.json")
	os.WriteFile(config, []byte("model = \"\"\nmodel_provider = \"workbench\"\n"), 0600)
	os.WriteFile(settings, []byte(`{"defaultModelSelection":{"instanceId":"codex","model":""},"textGenerationModelSelection":{"instanceId":"codex","model":"custom"},"other":true}`), 0600)
	if err := repairEmptyConsoleModel(config, settings, "approved"); err != nil {
		t.Fatal(err)
	}
	a, _ := os.ReadFile(config)
	b, _ := os.ReadFile(settings)
	if !strings.Contains(string(a), `model = "approved"`) || !strings.Contains(string(b), `"model":"approved"`) || !strings.Contains(string(b), `"model":"custom"`) || !strings.Contains(string(b), `"other":true`) {
		t.Fatal("migration lost configuration")
	}
	if err := repairEmptyConsoleModel(config, settings, "other"); err != nil {
		t.Fatal(err)
	}
	c, _ := os.ReadFile(config)
	if string(c) != string(a) {
		t.Fatal("overwrote selected model")
	}
}
