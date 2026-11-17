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

func TestDeleteRefreshesStaleStoppedMetadata(t *testing.T) {
	s, rt := setup(t)
	ws := create(t, s, "delete-stale")
	request(t, s, "POST", "/v1/workspaces/"+ws.ID+"/start", `{"idempotency_key":"start-stale"}`)
	ws, _ = s.Store.Workspace(t.Context(), ws.ID)
	ws.State = "STOPPED"
	if err := s.Store.Save(t.Context(), ws); err != nil {
		t.Fatal(err)
	}
	rt.jobs = 1
	path := "/v1/workspaces/" + ws.ID + "/delete"
	payload := fmt.Sprintf(`{"confirm_id":%q}`, ws.ID)
	if res := request(t, s, "POST", path, payload); res.Code == 200 {
		t.Fatal("deleted busy runtime with stale metadata")
	}
	if _, err := s.Store.Workspace(t.Context(), ws.ID); err != nil {
		t.Fatal(err)
	}
	rt.jobs = 0
	if res := request(t, s, "POST", path, payload); res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	if rt.managers[ws.ID] != nil {
		t.Fatal("deleted workspace still running")
	}
}

func TestHumanProjectAdminDeletion(t *testing.T) {
	for _, sharedMode := range []bool{false, true} {
		t.Run(fmt.Sprintf("shared=%t", sharedMode), func(t *testing.T) {
			s, _ := setup(t)
			if sharedMode {
				s.cfg.SharedWorkspaceID = "ws_shared"
				if _, err := s.EnsureSharedWorkspace(t.Context()); err != nil {
					t.Fatal(err)
				}
			}
			project := domainProject(t, s, "human-delete")
			path := "/v1/projects/" + project.ID
			for _, person := range []struct{ email, role string }{{"admin@example.com", "admin"}, {"member@example.com", "member"}} {
				body := fmt.Sprintf(`{"email":%q,"role":%q,"revision":0,"action":"invite"}`, person.email, person.role)
				if got := request(t, s, "POST", path+"/members", body); got.Code != 200 {
					t.Fatal(got.Code, got.Body)
				}
				if got := humanRequest(t, s, person.email, "POST", path+"/accept-invitation", `{"revision":1}`); got.Code != 200 {
					t.Fatal(got.Code, got.Body)
				}
			}
			body := fmt.Sprintf(`{"confirm_id":%q}`, project.ID)
			for _, email := range []string{"member@example.com", "outsider@example.com"} {
				got := humanRequest(t, s, email, "POST", path+"/delete", body)
				if got.Code != 403 && got.Code != 404 {
					t.Fatalf("unauthorized deletion: %d %s", got.Code, got.Body)
				}
			}
			if got := humanRequest(t, s, "admin@example.com", "POST", path+"/delete", `{"confirm_id":"wrong"}`); got.Code != 400 {
				t.Fatal("confirmation bypassed", got.Code, got.Body)
			}
			if got := humanRequest(t, s, "admin@example.com", "POST", path+"/delete", body); got.Code != 200 {
				t.Fatal("admin deletion denied", got.Code, got.Body)
			}
			if _, err := s.Store.Project(t.Context(), project.ID); err == nil {
				t.Fatal("project retained")
			}
			if sharedMode {
				if _, err := s.Store.Workspace(t.Context(), "ws_shared"); err != nil {
					t.Fatal("shared worker removed", err)
				}
			}
		})
	}
}
