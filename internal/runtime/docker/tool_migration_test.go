package docker

import "testing"

func TestRuntimeRejectsRetiredToolSelectionFields(t *testing.T) {
	c, engine := controller(t, false)
	for _, body := range []string{`{"home_id":"home_test","mcps":[]}`, `{"home_id":"home_test","profile_id":"general"}`} {
		response := controllerRequest(c, "POST", "/v1/workspaces/ws_test/start", c.cfg.Token, body)
		if response.Code != 400 || engine.createCount != 0 {
			t.Fatal("retired tool configuration allocated compute", response.Code)
		}
	}
}
