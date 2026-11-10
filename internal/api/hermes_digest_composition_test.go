package api

import (
	"agent-workbench/internal/mcpserver"
	"agent-workbench/internal/shared"
	"context"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPinnedHermesDigestPublishesThroughRealMCPAndWorkspace(t *testing.T) {
	source := os.Getenv("HERMES_SOURCE")
	if source == "" {
		t.Skip("requires pinned Hermes environment; set HERMES_SOURCE")
	}
	s, rt := setup(t)
	project := domainProject(t, s, "digest-personal")
	ws := domainWorkspace(t, s, "control-token", project.ID, "digest-home")
	binding := request(t, s, "POST", "/v1/projects/"+project.ID+"/coordination", `{"workspace_id":"`+ws.ID+`","notes_enabled":true}`)
	if binding.Code != 200 {
		t.Fatal(binding.Code, binding.Body.String())
	}
	cp := httptest.NewServer(s.Handler())
	defer cp.Close()
	handler, err := mcpserver.New(mcpserver.Config{ControlplaneURL: cp.URL, ControlToken: s.cfg.Token, MCPToken: "separate-mcp-credential", AllowedHosts: []string{"127.0.0.1"}})
	if err != nil {
		t.Fatal(err)
	}
	mcp := httptest.NewServer(handler)
	defer mcp.Close()
	grant, err := shared.IssueProjectAccess(s.cfg.Token, project.ID, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	script, err := filepath.Abs("../../scripts/test-hermes-digest-live.py")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(source, ".venv/bin/python"), script)
	cmd.Env = append(os.Environ(), "TEST_MCP_ENDPOINT="+mcp.URL+"/project-mcp", "TEST_MCP_GRANT="+grant, "TEST_PROJECT_ID="+project.ID, "TEST_WORKSPACE_ID="+ws.ID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("real digest failed: %s %v", output, err)
	}
	note, err := os.ReadFile(filepath.Join(rt.base, ws.ID, "files", "coordination", "notes", "digest-task-fixture-one.md"))
	if err != nil || !strings.Contains(string(note), "Sourced summary") || !strings.Contains(string(note), "https://example.org/paper") {
		t.Fatalf("missing sourced note: %s %v", note, err)
	}
}
