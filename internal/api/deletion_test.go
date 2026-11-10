package api

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDeleteWorkspaceAndEntireProject(t *testing.T) {
	s, _ := setup(t)
	project := domainProject(t, s, "delete-project")
	ws := domainWorkspace(t, s, "control-token", project.ID, "delete-ws")
	res := request(t, s, "POST", "/v1/workspaces/"+ws.ID+"/delete", `{"confirm_id":"wrong"}`)
	if res.Code != 400 {
		t.Fatalf("confirmation %d", res.Code)
	}
	path := "/v1/projects/" + project.ID + "/delete"
	req := httptest.NewRequest("POST", path, strings.NewReader(fmt.Sprintf(`{"confirm_id":%q}`, project.ID)))
	req.Header.Set("Authorization", "Bearer invalid-project-key")
	out := httptest.NewRecorder()
	s.Handler().ServeHTTP(out, req)
	if out.Code != 401 {
		t.Fatalf("unauthorized %d", out.Code)
	}
	res = request(t, s, "POST", path, fmt.Sprintf(`{"confirm_id":%q}`, project.ID))
	if res.Code != 200 {
		t.Fatalf("delete %d %s", res.Code, res.Body)
	}
	if res = request(t, s, "GET", "/v1/workspaces/"+ws.ID, ""); res.Code != 404 {
		t.Fatalf("deleted workspace accessible %d", res.Code)
	}
	if _, err := s.Store.Project(t.Context(), project.ID); err == nil {
		t.Fatal("deleted project accessible")
	}
	if _, err := s.Store.Workspace(t.Context(), ws.ID); err == nil {
		t.Fatal("deleted workspace accessible")
	}
}

func TestDeleteWorkspaceRequiresConfirmedIdleStop(t *testing.T) {
	s, rt := setup(t)
	ws := create(t, s, "delete-running")
	request(t, s, "POST", "/v1/workspaces/"+ws.ID+"/start", `{"idempotency_key":"delete-start"}`)
	path := "/v1/workspaces/" + ws.ID + "/delete"
	payload := fmt.Sprintf(`{"confirm_id":%q}`, ws.ID)
	rt.unknown = true
	if res := request(t, s, "POST", path, payload); res.Code == 200 {
		t.Fatal("unknown compute deleted")
	}
	if _, err := s.Store.Workspace(t.Context(), ws.ID); err != nil {
		t.Fatal("failed stop hid workspace")
	}
	rt.unknown = false
	if res := request(t, s, "POST", path, payload); res.Code != 200 {
		t.Fatalf("delete %d %s", res.Code, res.Body)
	}
	if _, err := s.Store.Workspace(t.Context(), ws.ID); err == nil {
		t.Fatal("workspace still accessible")
	}
}
