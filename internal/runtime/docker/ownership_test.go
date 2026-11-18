package docker

import (
	"agent-workbench/internal/runtime/protocol"
	"context"
	"strings"
	"testing"
)

func TestDockerProjectHomeBindingAndGrantIdentity(t *testing.T) {
	c, engine := controller(t, false)
	body := `{"project_id":"proj_alpha","home_id":"home_alpha","agent_kind":"codex","template_id":"development-v1"}`
	first := controllerRequest(c, "POST", "/v1/workspaces/ws_test/start", c.cfg.Token, body)
	if first.Code != 200 {
		t.Fatalf("start: %d %s", first.Code, first.Body)
	}
	labels := engine.spec["Labels"].(map[string]any)
	if labels["workbench.project"] != "proj_alpha" || labels["workbench.home"] != "home_alpha" {
		t.Fatal("ownership labels missing")
	}
	grant := c.token("ws_test", labels["workbench.generation"].(string))
	response := controllerRequest(c, "POST", "/v1/grants/verify", c.cfg.GatewayServiceToken, `{"token":"`+grant+`","audience":"inference"}`)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"project_id":"proj_alpha"`) {
		t.Fatal("grant identity lost project binding")
	}
	for _, body := range []string{`{"project_id":"proj_beta","home_id":"home_alpha","agent_kind":"codex"}`, `{"project_id":"proj_alpha","home_id":"home_beta","agent_kind":"codex"}`} {
		if controllerRequest(c, "POST", "/v1/workspaces/ws_test/start", c.cfg.Token, body).Code < 400 {
			t.Fatal("existing runtime reassigned")
		}
	}
	if engine.createCount != 1 {
		t.Fatal("ownership failure created another runtime")
	}
	if _, err := c.ensure(context.Background(), "ws_test", protocol.WorkspaceSpec{ProjectID: "proj_alpha", HomeID: "home_alpha", AgentKind: "hermes"}); err == nil {
		t.Fatal("unimplemented agent advertised as usable")
	}
}

func TestDefaultProjectRequiresHomeOwnership(t *testing.T) {
	c, engine := controller(t, false)
	for _, body := range []string{`{}`, `{"project_id":"default","home_id":"invalid/home"}`} {
		if response := controllerRequest(c, "POST", "/v1/workspaces/ws_test/start", c.cfg.Token, body); response.Code != 400 {
			t.Fatal(response.Code, response.Body)
		}
	}
	if engine.createCount != 0 {
		t.Fatal("invalid home allocated a runtime")
	}
	spec := protocol.WorkspaceSpec{ProjectID: "default", HomeID: "home_alpha"}
	for _, labels := range []map[string]string{
		{},
		{"workbench.project": "default"},
		{"workbench.home": "home_alpha"},
	} {
		if err := protocol.CheckOwnership(labels, spec); err == nil {
			t.Fatal("accepted incomplete ownership", labels)
		}
	}
	if err := protocol.CheckOwnership(map[string]string{"workbench.project": "default", "workbench.home": "home_alpha"}, spec); err != nil {
		t.Fatal(err)
	}
}
