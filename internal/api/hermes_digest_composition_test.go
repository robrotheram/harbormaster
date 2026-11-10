package api

import (
	"agent-workbench/internal/coordination"
	"agent-workbench/internal/mcpserver"
	"agent-workbench/internal/shared"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

func TestPinnedHermesConsumerCreatesOneProjectWorkAttempt(t *testing.T) {
	source := os.Getenv("HERMES_SOURCE")
	if source == "" {
		t.Skip("requires pinned Hermes environment; set HERMES_SOURCE")
	}
	s, rt := setup(t)
	personal, research := domainProject(t, s, "consumer-personal"), domainProject(t, s, "consumer-research")
	personalWS := domainWorkspace(t, s, "control-token", personal.ID, "personal-home")
	researchWS := domainWorkspace(t, s, "control-token", research.ID, "research-home")
	if got := request(t, s, "POST", "/v1/projects/"+personal.ID+"/coordination", `{"workspace_id":"`+personalWS.ID+`","notes_enabled":true}`); got.Code != 200 {
		t.Fatal(got.Code, got.Body.String())
	}
	if got := request(t, s, "POST", "/v1/workspaces/"+researchWS.ID+"/start", `{"idempotency_key":"start"}`); got.Code != 200 {
		t.Fatal(got.Code, got.Body.String())
	}
	instruction := []byte("Describe the workspace.")
	if err := os.WriteFile(filepath.Join(rt.base, researchWS.ID, "files", "research.md"), instruction, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(instruction)
	ctx := context.Background()
	grant := coordination.Grant{ID: "personal-to-research", Source: personal.ID, Destination: research.ID, Operation: "work-request"}
	if err := s.coordination.Create(ctx, personal.ID, grant); err != nil {
		t.Fatal(err)
	}
	if err := s.coordination.Approve(ctx, research.ID, grant.ID); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]string{"workspace_id": researchWS.ID, "specification_path": "research.md", "specification_digest": hex.EncodeToString(digest[:]), "kind": "research"})
	if _, err := s.coordination.Publish(ctx, personal.ID, grant.ID, "digest-task-one", payload); err != nil {
		t.Fatal(err)
	}
	cp := httptest.NewServer(s.Handler())
	defer cp.Close()
	handler, err := mcpserver.New(mcpserver.Config{ControlplaneURL: cp.URL, ControlToken: s.cfg.Token, MCPToken: "separate-mcp-credential", AllowedHosts: []string{"127.0.0.1"}})
	if err != nil {
		t.Fatal(err)
	}
	endpoint := httptest.NewServer(handler)
	defer endpoint.Close()
	token, err := shared.IssueProjectAccess(s.cfg.Token, research.ID, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	script, err := filepath.Abs("../../scripts/test-hermes-consume-live.py")
	if err != nil {
		t.Fatal(err)
	}
	processCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(processCtx, filepath.Join(source, ".venv/bin/python"), script)
	cmd.Env = append(os.Environ(), "TEST_MCP_ENDPOINT="+endpoint.URL+"/project-mcp", "TEST_MCP_GRANT="+token, "TEST_PROJECT_ID="+research.ID)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("real consumer failed: %s %v", output, err)
	}
	var count int
	if err = s.Store.DB.QueryRow(`SELECT count(*) FROM work_attempts WHERE project_id=$1`, research.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("consumer duplicated work", count, err)
	}
}
