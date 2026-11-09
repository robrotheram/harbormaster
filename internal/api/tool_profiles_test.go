package api

import (
	runtime "agent-workbench/internal/runtime/protocol"
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type toolProfileRuntime struct {
	*embeddedRuntime
	spec runtime.WorkspaceSpec
}

func (rt *toolProfileRuntime) StartConfigured(ctx context.Context, id string, spec runtime.WorkspaceSpec) (shared.Observation, error) {
	rt.spec = spec
	return rt.Start(ctx, id)
}
func TestCustomToolProfilesPersistAndConfigureRestarts(t *testing.T) {
	s, base := setup(t)
	rt := &toolProfileRuntime{embeddedRuntime: base}
	s.Runtime = rt
	body := `{"title":"Files only","description":"A reusable profile","mcps":["workspace-files"],"revision":0}`
	s.cfg.ProjectTokens = map[string]string{"project-agent": "default"}
	if w := domainRequest(t, s, "project-agent", "POST", "/v1/tool-profiles", body); w.Code != 403 {
		t.Fatal("global edit accepted", w.Code)
	}
	w := request(t, s, "POST", "/v1/tool-profiles", body)
	var p shared.Profile
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &p) != nil {
		t.Fatal(w.Code, w.Body)
	}
	for _, invalid := range []string{`["shell"]`, `["workspace-files","workspace-files"]`, `null`} {
		w := request(t, s, "POST", "/v1/tool-profiles", strings.Replace(body, `["workspace-files"]`, invalid, 1))
		if w.Code != 400 {
			t.Fatal("invalid MCP selection", w.Code)
		}
	}
	wsResponse := request(t, s, "POST", "/v1/workspaces", `{"project_id":"default","idempotency_key":"custom-tools","profile_id":"`+p.ID+`"}`)
	var ws shared.Workspace
	json.Unmarshal(wsResponse.Body.Bytes(), &ws)
	if wsResponse.Code != 201 {
		t.Fatal(wsResponse.Body)
	}
	if w := request(t, s, "POST", "/v1/workspaces/"+ws.ID+"/start", `{"idempotency_key":"start"}`); w.Code != 200 {
		t.Fatal(w.Body)
	}
	if len(rt.spec.MCPs) != 1 || rt.spec.MCPs[0] != "workspace-files" {
		t.Fatal("custom profile not passed to runtime", rt.spec)
	}
	update := `{"title":"No MCPs","description":"","mcps":[],"revision":1}`
	if w := request(t, s, "POST", "/v1/tool-profiles/"+p.ID, update); w.Code != 200 {
		t.Fatal(w.Body)
	}
	if len(rt.spec.MCPs) != 1 || base.starts != 1 {
		t.Fatal("edit interrupted running workspace")
	}
	if w := request(t, s, "POST", "/v1/tool-profiles/"+p.ID, update); w.Code != 409 {
		t.Fatal("stale edit accepted", w.Code)
	}
	restarted := New(s.Store, rt, s.cfg)
	w = request(t, restarted, "GET", "/v1/profiles", "")
	if !strings.Contains(w.Body.String(), "No MCPs") {
		t.Fatal("profile not persisted")
	}
	if w := request(t, restarted, "POST", "/v1/workspaces/"+ws.ID+"/stop", `{"idempotency_key":"stop"}`); w.Code != 200 {
		t.Fatal(w.Body)
	}
	if w := request(t, restarted, "POST", "/v1/workspaces/"+ws.ID+"/start", `{"idempotency_key":"again"}`); w.Code != 200 {
		t.Fatal(w.Body)
	}
	if rt.spec.MCPs == nil || len(rt.spec.MCPs) != 0 {
		t.Fatal("empty profile lost on restart")
	}
}
