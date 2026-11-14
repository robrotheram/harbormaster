package api

import "testing"

func TestTerminalOperatorAccessAndNoImplicitStart(t *testing.T) {
	s, rt := setup(t)
	ws := create(t, s, "terminal")
	s.cfg.ProjectTokens = map[string]string{"project-agent": "default"}
	path := "/v1/workspaces/" + ws.ID + "/terminals"
	body := `{"id":"terminal_test","cols":80,"rows":24}`
	if r := domainRequest(t, s, "project-agent", "POST", path, body); r.Code != 403 {
		t.Fatal("project agent obtained operator shell", r.Code)
	}
	if r := request(t, s, "POST", path, body); r.Code != 409 {
		t.Fatal("stopped worker", r.Code, r.Body.String())
	}
	if rt.starts != 0 {
		t.Fatal("terminal started compute")
	}
	if r := request(t, s, "POST", "/v1/workspaces/"+ws.ID+"/start", `{"idempotency_key":"terminal-start"}`); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	if r := request(t, s, "POST", path, body); r.Code != 201 {
		t.Fatal(r.Code, r.Body.String())
	}
	if r := domainRequest(t, s, "project-agent", "POST", path+"/terminal_test/exchange", `{"cols":80,"rows":24}`); r.Code != 403 {
		t.Fatal("project agent read operator shell")
	}
	if r := request(t, s, "POST", path+"/terminal_test/exchange", `{"cols":80,"rows":24}`); r.Code != 200 || r.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(r.Code, r.Body.String())
	}
	if r := request(t, s, "POST", path+"/terminal_test/close", `{}`); r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
}
