package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agent-workbench/internal/shared"
	"agent-workbench/internal/store"
)

func domainRequest(t *testing.T, s *Server, token, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("X-Workbench-Project", "forged-other-project")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}
func domainProject(t *testing.T, s *Server, name string) shared.Project {
	t.Helper()
	w := request(t, s, "POST", "/v1/projects", `{"name":"`+name+`","idempotency_key":"project-`+name+`"}`)
	if w.Code != 201 {
		t.Fatalf("project create: %d %s", w.Code, w.Body)
	}
	var p shared.Project
	if json.Unmarshal(w.Body.Bytes(), &p) != nil || p.ID == "" {
		t.Fatal("missing project")
	}
	return p
}
func domainWorkspace(t *testing.T, s *Server, token, project, key string) shared.Workspace {
	t.Helper()
	body := `{"project_id":"` + project + `","idempotency_key":"` + key + `","name":"private project workspace","agent_kind":"t3"}`
	w := domainRequest(t, s, token, "POST", "/v1/workspaces", body)
	if w.Code != 201 && w.Code != 200 {
		t.Fatalf("workspace create: %d %s", w.Code, w.Body)
	}
	var ws shared.Workspace
	if json.Unmarshal(w.Body.Bytes(), &ws) != nil || ws.HomeID == "" {
		t.Fatal("workspace has no private home")
	}
	return ws
}
func TestProjectCredentialsFenceEveryWorkspaceRoute(t *testing.T) {
	s, rt := setup(t)
	a, b := domainProject(t, s, "A"), domainProject(t, s, "B")
	s.cfg.ProjectTokens = map[string]string{"project-a-credential": a.ID, "project-b-credential": b.ID}
	wa := domainWorkspace(t, s, "project-a-credential", a.ID, "same-create-key")
	wb := domainWorkspace(t, s, "project-b-credential", b.ID, "same-create-key")
	if wa.ID == wb.ID || wa.HomeID == wb.HomeID {
		t.Fatal("cross-project idempotency or home collision")
	}
	if domainWorkspace(t, s, "project-a-credential", a.ID, "same-create-key").ID != wa.ID {
		t.Fatal("creation retry duplicated workspace")
	}
	for _, tt := range []struct{ method, path, body string }{
		{"GET", "/v1/workspaces/" + wb.ID, ""},
		{"POST", "/v1/workspaces/" + wb.ID + "/start", `{"idempotency_key":"deny"}`},
		{"POST", "/v1/workspaces/" + wb.ID + "/stop", `{"idempotency_key":"deny"}`},
		{"POST", "/v1/workspaces/" + wb.ID + "/settings", `{"idle_timeout_minutes":60,"connections":{"mode":"open","destinations":[]}}`},
		{"POST", "/v1/workspaces/" + wb.ID + "/enable-t3", `{"idempotency_key":"deny"}`},
		{"GET", "/v1/workspaces/" + wb.ID + "/connection", ""},
		{"GET", "/v1/workspaces/" + wb.ID + "/console/api/auth/session", ""},
		{"GET", "/v1/workspaces/" + wb.ID + "/desktop/.well-known/t3/environment", ""},
		{"POST", "/v1/workspaces/" + wb.ID + "/tasks", `{"instruction":"denied","idempotency_key":"deny"}`},
		{"GET", "/v1/workspaces/" + wb.ID + "/tasks/task_test", ""},
		{"GET", "/v1/workspaces/" + wb.ID + "/tasks/task_test/result", ""},
		{"POST", "/v1/workspaces/" + wb.ID + "/tasks/task_test/cancel", `{"idempotency_key":"deny"}`},
	} {
		w := domainRequest(t, s, "project-a-credential", tt.method, tt.path, tt.body)
		if w.Code != 404 {
			t.Fatalf("cross-project %s %s: %d", tt.method, tt.path, w.Code)
		}
	}
	force := domainRequest(t, s, "project-a-credential", "POST", "/v1/workspaces/"+wa.ID+"/force-stop", `{"idempotency_key":"deny"}`)
	if force.Code != 403 {
		t.Fatalf("project credential forced a workspace stop: %d", force.Code)
	}
	settings := domainRequest(t, s, "project-a-credential", "POST", "/v1/workspaces/"+wa.ID+"/settings", `{"idle_timeout_minutes":60,"connections":{"mode":"open","destinations":[]}}`)
	if settings.Code != 403 {
		t.Fatalf("project credential changed inline policy: %d", settings.Code)
	}
	if rt.starts != 0 {
		t.Fatal("denied operations started a workspace")
	}
	for _, route := range []string{"/v1/workspaces", "/v1/projects"} {
		w := domainRequest(t, s, "project-a-credential", "GET", route, "")
		if w.Code != 200 {
			t.Fatalf("scoped list: %d", w.Code)
		}
		if strings.Contains(w.Body.String(), b.ID) || strings.Contains(w.Body.String(), wb.ID) || strings.Contains(w.Body.String(), wb.HomeID) {
			t.Fatal("foreign metadata leaked through " + route)
		}
		if !strings.Contains(w.Body.String(), a.ID) {
			t.Fatal("own project absent from " + route)
		}
	}
	for _, route := range []string{"/v1/workspaces", "/v1/usage"} {
		if w := domainRequest(t, s, "project-a-credential", "GET", route+"?project_id="+b.ID, ""); w.Code != 404 {
			t.Fatal("foreign project filter accepted")
		}
	}
	if w := domainRequest(t, s, "project-a-credential", "POST", "/v1/projects", `{"name":"admin escalation","idempotency_key":"deny"}`); w.Code != 403 {
		t.Fatal("project token created project")
	}
	if w := domainRequest(t, s, "project-a-credential", "POST", "/v1/workspaces", `{"project_id":"`+b.ID+`","idempotency_key":"deny"}`); w.Code != 404 {
		t.Fatal("project argument widened authority")
	}
}
func TestHumanCannotSeedOrHandoffAcrossProjects(t *testing.T) {
	s, rt := setup(t)
	a, b := domainProject(t, s, "A"), domainProject(t, s, "B")
	wa := domainWorkspace(t, s, "control-token", a.ID, "a")
	wb := domainWorkspace(t, s, "control-token", b.ID, "b")
	req := `{"project_id":"` + b.ID + `","parent_id":"` + wa.ID + `","branch":"pr-1","idempotency_key":"cross-seed"}`
	if w := request(t, s, "POST", "/v1/workspaces", req); w.Code != 404 {
		t.Fatal("human cross-project seed accepted")
	}
	req = `{"source_id":"` + wa.ID + `","source_path":"report.md","destination_id":"` + wb.ID + `","destination_path":"report.md"}`
	if w := request(t, s, "POST", "/v1/file-handoffs", req); w.Code != 403 {
		t.Fatalf("human cross-project handoff: %d %s", w.Code, w.Body)
	}
	if rt.starts != 0 {
		t.Fatal("denial touched runtime")
	}
	changed := wa
	changed.ProjectID = b.ID
	if err := s.Store.Save(context.Background(), changed); err != store.ErrOwnership {
		t.Fatal("workspace project was mutable")
	}
	changed = wa
	changed.HomeID = wb.HomeID
	if err := s.Store.Save(context.Background(), changed); err != store.ErrOwnership {
		t.Fatal("workspace home was mutable")
	}
}
func TestProjectUsageDoesNotIncludeOtherProjects(t *testing.T) {
	s, _ := setup(t)
	a, b := domainProject(t, s, "A"), domainProject(t, s, "B")
	s.cfg.ProjectTokens = map[string]string{"project-a-credential": a.ID}
	wa := domainWorkspace(t, s, "control-token", a.ID, "a")
	wb := domainWorkspace(t, s, "control-token", b.ID, "b")
	for _, id := range []string{wa.ID, wb.ID} {
		if w := request(t, s, "POST", "/v1/workspaces/"+id+"/start", `{"idempotency_key":"start"}`); w.Code != 200 {
			t.Fatal("start failed")
		}
	}
	w := domainRequest(t, s, "project-a-credential", "GET", "/v1/usage", "")
	var usage struct {
		Count int `json:"running_workspaces"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &usage) != nil || usage.Count != 1 {
		t.Fatalf("usage leaked: %s", w.Body)
	}
}

func TestIssuedProjectAccessCannotEscalateOrSurviveExpiry(t *testing.T) {
	s, _ := setup(t)
	a, b := domainProject(t, s, "A"), domainProject(t, s, "B")
	wa := domainWorkspace(t, s, "control-token", a.ID, "signed-a")
	wb := domainWorkspace(t, s, "control-token", b.ID, "signed-b")
	minted := request(t, s, "POST", "/v1/projects/"+a.ID+"/agent-access", `{"ttl_seconds":3600}`)
	var access struct {
		Token string `json:"access_token"`
	}
	if minted.Code != 201 || json.Unmarshal(minted.Body.Bytes(), &access) != nil || access.Token == "" {
		t.Fatal("project access not issued")
	}
	if w := domainRequest(t, s, access.Token, "GET", "/v1/workspaces/"+wa.ID, ""); w.Code != 200 {
		t.Fatal("own signed access denied")
	}
	if w := domainRequest(t, s, access.Token, "GET", "/v1/workspaces/"+wb.ID, ""); w.Code != 404 {
		t.Fatal("signed access crossed project")
	}
	if w := domainRequest(t, s, access.Token, "POST", "/v1/projects/"+b.ID+"/agent-access", `{}`); w.Code != 403 {
		t.Fatal("project credential minted another credential")
	}
	for _, ttl := range []string{"1", "-1", "604801"} {
		if w := request(t, s, "POST", "/v1/projects/"+a.ID+"/agent-access", `{"ttl_seconds":`+ttl+`}`); w.Code != 400 {
			t.Fatal("unbounded credential TTL accepted")
		}
	}
	for _, token := range []string{access.Token + "tampered", access.Token[:len(access.Token)-1]} {
		if w := domainRequest(t, s, token, "GET", "/v1/workspaces", ""); w.Code != 401 {
			t.Fatal("forged credential accepted")
		}
	}
	expired, err := shared.IssueProjectAccess(s.cfg.Token, a.ID, s.now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if w := domainRequest(t, s, expired, "GET", "/v1/projects", ""); w.Code != 401 {
		t.Fatal("expired project credential accepted")
	}
}
