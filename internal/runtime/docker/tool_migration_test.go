package docker

import (
	"bytes"
	"strings"
	"testing"
)

func TestRetiredToolSelectionMigratesOnlyStoppedContainer(t *testing.T) {
	c, engine := controller(t, false)
	start := func() {
		t.Helper()
		response := controllerRequest(c, "POST", "/v1/workspaces/ws_test/start", c.cfg.Token, `{"agent_kind":"codex"}`)
		if response.Code != 200 {
			t.Fatal(response.Code, response.Body.String())
		}
	}
	stop := func() {
		t.Helper()
		response := controllerRequest(c, "POST", "/v1/workspaces/ws_test/stop", c.cfg.Token, "")
		if response.Code != 200 {
			t.Fatal(response.Code, response.Body.String())
		}
	}
	start()
	volume := mustJSON(engine.spec["HostConfig"].(map[string]any)["Mounts"])
	engine.spec["Env"] = append(engine.spec["Env"].([]any), "WORKSPACE_MCPS=[]")
	engine.spec["Labels"].(map[string]any)["workbench.profile"] = "retired-profile"
	start()
	if engine.removed != 0 {
		t.Fatal("tool migration replaced a running workspace")
	}
	stop()
	start()
	if engine.removed != 1 || engine.createCount != 2 || !bytes.Equal(volume, mustJSON(engine.spec["HostConfig"].(map[string]any)["Mounts"])) {
		t.Fatal("stopped migration did not retain the existing private volume")
	}
	for _, value := range engine.spec["Env"].([]any) {
		if strings.HasPrefix(value.(string), "WORKSPACE_MCPS=") {
			t.Fatal("new container retained retired selection configuration")
		}
	}
	if _, exists := engine.spec["Labels"].(map[string]any)["workbench.profile"]; exists {
		t.Fatal("new container retained a profile label")
	}
	stop()
	start()
	if engine.removed != 1 || engine.createCount != 2 {
		t.Fatal("tool migration kept replacing current containers")
	}
}

func TestRuntimeRejectsRetiredToolSelectionFields(t *testing.T) {
	c, engine := controller(t, false)
	for _, body := range []string{`{"mcps":[]}`, `{"profile_id":"general"}`} {
		response := controllerRequest(c, "POST", "/v1/workspaces/ws_test/start", c.cfg.Token, body)
		if response.Code != 400 || engine.createCount != 0 {
			t.Fatal("retired tool configuration allocated compute", response.Code)
		}
	}
}

func TestExplicitConsoleUpgradeStartsWithoutProfileMarker(t *testing.T) {
	c, engine := controller(t, false)
	response := controllerRequest(c, "POST", "/v1/workspaces/ws_test/start", c.cfg.Token, `{"agent_kind":"codex","console_enabled":true}`)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	found := false
	for _, value := range engine.spec["Env"].([]any) {
		found = found || value == "WORKSPACE_T3=true"
	}
	if !found {
		t.Fatal("explicit native console upgrade was not retained on cold start")
	}
}
