package api

import (
	"testing"

	"agent-workbench/internal/store"
)

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

func TestProjectAdminCanManageWorkerTerminal(t *testing.T) {
	s, _ := setup(t)
	ws := create(t, s, "project-terminal")
	for _, member := range []store.ProjectMember{
		{Email: "admin@example.com", UserID: "user_admin_example_com", Role: "admin"},
		{Email: "member@example.com", UserID: "user_member_example_com", Role: "member"},
	} {
		member.ProjectID = ws.ProjectID
		member.Status = "active"
		if err := s.Store.SaveMember(t.Context(), member, 0, "", true); err != nil {
			t.Fatal(err)
		}
	}

	worker := "/v1/workspaces/" + ws.ID
	if r := request(t, s, "POST", worker+"/start", `{"idempotency_key":"project-terminal-start"}`); r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}

	for _, action := range []struct {
		path string
		body string
		want int
	}{
		{"/terminals", `{"id":"terminal_test","cols":80,"rows":24}`, 201},
		{"/terminals/terminal_test/exchange", `{"cols":80,"rows":24}`, 200},
		{"/terminals/terminal_test/close", `{}`, 200},
	} {
		t.Run(action.path, func(t *testing.T) {
			path := worker + action.path
			if r := humanRequest(t, s, "member@example.com", "POST", path, action.body); r.Code != 403 {
				t.Fatal("project member", r.Code, r.Body.String())
			}
			if r := humanRequest(t, s, "outsider@example.com", "POST", path, action.body); r.Code != 404 {
				t.Fatal("nonmember", r.Code, r.Body.String())
			}
			if r := humanRequest(t, s, "admin@example.com", "POST", path, action.body); r.Code != action.want {
				t.Fatal("project admin", r.Code, r.Body.String())
			}
		})
	}
}
