package workspace

import (
	"agent-workbench/internal/shared"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestModelsFilterInstalledAgentsAndReportWorkerDefault(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "codex"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer test-workspace-token" {
			t.Error("incorrect catalog authority")
		}
		shared.JSON(w, 200, shared.AgentModels{Items: []shared.AgentModel{{Provider: "codex", Model: "available-model"}, {Provider: "claude", Model: "not-installed"}, {Provider: "codex", Model: "bad model"}}})
	}))
	defer gateway.Close()
	m, err := New(Config{ID: "ws_test", Token: "test-workspace-token", DataDir: t.TempDir(), InferenceURL: gateway.URL, DefaultProvider: "codex", CodexModel: "available-model"})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	w := invoke(t, m.Handler(), "GET", "/v1/models", "")
	var result shared.AgentModels
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	if result.WorkspaceID != "ws_test" || result.DefaultModel != "available-model" || len(result.Items) != 1 || result.Items[0].Provider != "codex" {
		t.Fatal(result)
	}
}
