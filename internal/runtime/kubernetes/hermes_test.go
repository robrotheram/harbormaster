package kubernetes

import (
	"agent-workbench/internal/mcpserver"
	"agent-workbench/internal/runtime/protocol"
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestHermesApprovalAndGenerationMCP(t *testing.T) {
	k, f := kubeProvider(t)
	spec := protocol.WorkspaceSpec{ProjectID: "proj_one", HomeID: "home_one", AgentKind: "hermes"}
	if _, err := k.StartConfigured(context.Background(), "ws_one", spec); err == nil {
		t.Fatal("unapproved Hermes admitted")
	}
	k.cfg.HermesApprovedImage = k.cfg.Image
	k.cfg.HermesModel = "approved-model"
	if !k.hermesEnabled() {
		t.Fatal("approved image unavailable")
	}
	k.cfg.RequireVerified = true
	if _, err := k.StartConfigured(context.Background(), "ws_one", spec); err == nil {
		t.Fatal("verified isolation bypass")
	}
	k.cfg.RequireVerified = false
	labels := map[string]string{"workbench.deployment": k.cfg.Deployment, "workbench.workspace": "ws_one", "workbench.project": "proj_one", "workbench.home": "home_one", "workbench.agent": "hermes", "workbench.generation": "gen_one"}
	body := k.sandbox("ws_one", spec, "project-one", labels, "grant")
	// Roundtrip matches the API's unstructured representation.
	raw, _ := json.Marshal(body)
	json.Unmarshal(raw, &body)
	body["metadata"].(map[string]any)["generation"] = float64(1)
	body["status"] = map[string]any{"conditions": []any{map[string]any{"type": "Ready", "status": "True", "observedGeneration": float64(1)}}}
	f.resources["project-one/"+k.name("ws_one")] = body
	controller := httptest.NewServer(k.Handler())
	defer controller.Close()
	var acceptedCalls atomic.Int32
	cp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		project, err := shared.VerifyProjectAccess(strings.Repeat("s", 32), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), time.Now())
		if err != nil || project != "proj_one" {
			t.Error("project authority escaped verified metadata")
		}
		if r.URL.Query().Get("project_id") != project {
			shared.Error(w, http.StatusNotFound, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		acceptedCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[]}`))
	}))
	defer cp.Close()
	handler, err := mcpserver.New(mcpserver.Config{ControlplaneURL: cp.URL, ControlToken: strings.Repeat("s", 32), MCPToken: strings.Repeat("m", 32), ControllerURL: controller.URL, GatewayServiceToken: k.cfg.GatewayServiceToken, AllowedHosts: []string{"example.test"}})
	if err != nil {
		t.Fatal(err)
	}
	invoke := func(token, method string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://example.test/hermes-mcp", strings.NewReader(method))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json, text/event-stream")
		r.Header.Set("X-Project-ID", "proj_two")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	grant := k.token("ws_one", "gen_one")
	w := invoke(grant, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "work_create") || strings.Contains(w.Body.String(), "work_review") {
		t.Fatalf("MCP tools: %d %s", w.Code, w.Body.String())
	}
	w = invoke(grant, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"work_list","arguments":{"project_id":"proj_one"}}}`)
	if w.Code != http.StatusOK || acceptedCalls.Load() != 1 {
		t.Fatalf("own project invocation failed: %d %s", w.Code, w.Body.String())
	}
	w = invoke(grant, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"work_list","arguments":{"project_id":"proj_two"}}}`)
	var response struct {
		Result mcp.CallToolResult `json:"result"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &response) != nil || !response.Result.IsError {
		t.Fatalf("foreign project invocation did not return an MCP tool error: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "NOT_FOUND_OR_FORBIDDEN") || acceptedCalls.Load() != 1 {
		t.Fatalf("foreign project invocation was not denied by project scope: %s", w.Body.String())
	}
	for _, token := range []string{k.token("ws_foreign", "gen_one"), k.token("ws_one", "gen_old")} {
		if w := invoke(token, `{"method":"tools/list"}`); w.Code != 401 {
			t.Fatalf("foreign/revoked: %d", w.Code)
		}
	}
	if w := invoke(grant, `{"method":"tools/call","params":{"name":"work_review"}}`); w.Code != 403 {
		t.Fatal("admin tool accepted")
	}
	body["spec"].(map[string]any)["operatingMode"] = "Suspended"
	if w := invoke(grant, `{"method":"tools/list"}`); w.Code != 401 {
		t.Fatal("suspended grant accepted")
	}
}
