package api

import (
	"agent-workbench/internal/shared"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func sampleForOverview(t *testing.T, s *Server) (shared.Project, []boardCard) {
	t.Helper()
	res := humanRequest(t, s, "alice@example.com", "POST", "/v1/projects", `{"name":"Sample","idempotency_key":"overview","sample":true}`)
	var p shared.Project
	if res.Code != 201 || json.Unmarshal(res.Body.Bytes(), &p) != nil {
		t.Fatal(res.Code, res.Body)
	}
	res = humanRequest(t, s, "alice@example.com", "GET", "/v1/board?project_id="+p.ID, "")
	var list struct {
		Items []boardCard `json:"items"`
	}
	if res.Code != 200 || json.Unmarshal(res.Body.Bytes(), &list) != nil || len(list.Items) != 3 {
		t.Fatal(res.Code, res.Body)
	}
	return p, list.Items
}
func TestOverviewReferencesAssignmentsAndSharedModeCapabilities(t *testing.T) {
	s, _ := setup(t)
	s.cfg.SharedWorkspaceID = "ws_shared"
	p, cards := sampleForOverview(t, s)
	gotCreate := humanRequest(t, s, "alice@example.com", "POST", "/v1/board", fmt.Sprintf(`{"project_id":%q,"title":"New planned task","description":"A plan, not a run","kind":"software","idempotency_key":"plan-without-worker"}`, p.ID))
	if gotCreate.Code != 201 {
		t.Fatal("planning blocked in shared mode", gotCreate.Code, gotCreate.Body)
	}
	workers, err := s.Store.Workspaces(t.Context())
	if err != nil || len(workers) != 0 {
		t.Fatal("planning allocated workers", workers, err)
	}

	seen := map[int]bool{}
	for _, card := range cards {
		if card.Number < 1 || seen[card.Number] {
			t.Fatal("invalid reference", card)
		}
		seen[card.Number] = true
	}
	c := cards[0]
	path := "/v1/board/" + c.ID
	for _, owner := range []string{"user_foreign", "user_invited"} {
		body := fmt.Sprintf(`{"project_id":%q,"revision":%d,"stage":"planned","owner_id":%q}`, p.ID, c.Revision, owner)
		if got := humanRequest(t, s, "alice@example.com", "POST", path, body); got.Code != 400 {
			t.Fatal("invalid assignment accepted", got.Code, got.Body)
		}
	}
	body := fmt.Sprintf(`{"project_id":%q,"revision":%d,"stage":"planned","owner_id":"user_alice_example_com","title":"Edited together","description":"New description"}`, p.ID, c.Revision)
	got := humanRequest(t, s, "alice@example.com", "POST", path, body)
	var updated boardCard
	if got.Code != 200 || json.Unmarshal(got.Body.Bytes(), &updated) != nil || updated.OwnerID != "user_alice_example_com" || updated.Owner != "alice@example.com" || updated.Number != c.Number || updated.Title != "Edited together" || *updated.Description != "New description" {
		t.Fatal(got.Code, got.Body)
	}
	if got = humanRequest(t, s, "alice@example.com", "POST", path, body); got.Code != 409 {
		t.Fatal("stale edit accepted", got.Code, got.Body)
	}
	got = humanRequest(t, s, "alice@example.com", "GET", "/v1/projects/"+p.ID+"/capabilities", "")
	var caps struct {
		CanStart  bool `json:"can_start"`
		CanImport bool `json:"can_import"`
	}
	if got.Code != 200 || json.Unmarshal(got.Body.Bytes(), &caps) != nil || caps.CanStart || caps.CanImport {
		t.Fatal(got.Code, got.Body)
	}
	if got = humanRequest(t, s, "other@example.com", "GET", "/v1/projects/"+p.ID+"/capabilities", ""); got.Code != 404 {
		t.Fatal("foreign capabilities exposed", got.Code)
	}
	if got = humanRequest(t, s, "alice@example.com", "POST", path+"/start", fmt.Sprintf(`{"project_id":%q,"revision":2}`, p.ID)); got.Code != 409 {
		t.Fatal("shared execution allowed", got.Code, got.Body)
	}
}

func TestSubscriptionTaskStartsOnSelectedWorkerAndModel(t *testing.T) {
	s, runtime := setup(t)
	s.Runtime = &modelCatalogRuntime{embeddedRuntime: runtime}
	s.cfg.RequireAgentSelection = true
	project, cards := sampleForOverview(t, s)
	card := cards[0]
	path := "/v1/board/" + card.ID + "/start"
	body := fmt.Sprintf(`{"project_id":%q,"revision":%d}`, project.ID, card.Revision)
	if got := request(t, s, "POST", path, body); got.Code != 409 {
		t.Fatal(got.Code, got.Body)
	}
	workers, err := s.Store.Workspaces(t.Context())
	if err != nil || len(workers) != 0 {
		t.Fatal("start without a selection allocated a worker", workers, err)
	}
	worker := domainWorkspace(t, s, "control-token", project.ID, "subscription-worker")
	body = fmt.Sprintf(`{"project_id":%q,"revision":%d,"workspace_id":%q,"selection":{"provider":"codex","model":"first-model"}}`, project.ID, card.Revision, worker.ID)
	got := request(t, s, "POST", path, body)
	var attempt shared.WorkAttempt
	if got.Code != 202 || json.Unmarshal(got.Body.Bytes(), &attempt) != nil || attempt.WorkspaceID != worker.ID || attempt.Model != "first-model" {
		t.Fatal(got.Code, got.Body)
	}
}

func TestStartPlannedTaskRetainsIdentityAndRetriesOneAttempt(t *testing.T) {
	s, _ := setup(t)
	p, cards := sampleForOverview(t, s)
	c := cards[0]
	path := "/v1/board/" + c.ID + "/start"
	body := fmt.Sprintf(`{"project_id":%q,"revision":%d}`, p.ID, c.Revision)
	got := humanRequest(t, s, "foreign@example.com", "POST", path, body)
	if got.Code != 404 {
		t.Fatal("foreign start", got.Code, got.Body)
	}
	got = humanRequest(t, s, "alice@example.com", "POST", path, fmt.Sprintf(`{"project_id":%q,"revision":0}`, p.ID))
	if got.Code != 409 {
		t.Fatal("stale start", got.Code, got.Body)
	}
	if got := humanRequest(t, s, "alice@example.com", "POST", path, body); got.Code != 409 || !strings.Contains(got.Body.String(), "WORKER_SELECTION_REQUIRED") {
		t.Fatal("unassigned start did not require a worker", got.Code, got.Body)
	}
	workers, err := s.Store.Workspaces(t.Context())
	if err != nil || len(workers) != 0 {
		t.Fatal("unassigned start allocated a worker", workers, err)
	}
	worker := domainWorkspace(t, s, "control-token", p.ID, "overview-worker")
	body = fmt.Sprintf(`{"project_id":%q,"revision":%d,"workspace_id":%q}`, p.ID, c.Revision, worker.ID)
	var receipt shared.WorkAttempt
	for i := 0; i < 2; i++ {
		got = humanRequest(t, s, "alice@example.com", "POST", path, body)
		var a shared.WorkAttempt
		if got.Code != 202 && got.Code != 200 {
			t.Fatal("start", got.Code, got.Body)
		}
		if json.Unmarshal(got.Body.Bytes(), &a) != nil || a.WorkItemID != c.ID || a.WorkspaceID != worker.ID {
			t.Fatal("bad attempt", got.Body)
		}
		if i == 1 && receipt.ID != a.ID {
			t.Fatal("retry duplicated run")
		}
		receipt = a
	}
	var count int
	if err := s.Store.DB.QueryRow(`SELECT COUNT(*) FROM work_items WHERE project_id=$1`, p.ID).Scan(&count); err != nil || count != 3 {
		t.Fatal("duplicated task", count, err)
	}
	if err := s.Store.DB.QueryRow(`SELECT COUNT(*) FROM work_attempts WHERE project_id=$1`, p.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicated attempt", count, err)
	}
	saved, err := s.Store.BoardItem(t.Context(), p.ID, c.ID)
	if err != nil || saved.Title != c.Title || saved.Number != c.Number {
		t.Fatal("lost task metadata", saved, err)
	}
	got = humanRequest(t, s, "alice@example.com", "GET", "/v1/work-items/"+c.ID+"/instruction?project_id="+p.ID, "")
	if got.Code != 200 {
		t.Fatal("instruction not prepared", got.Code, got.Body)
	}
}

func TestRequiredDependenciesGateDispatch(t *testing.T) {
	s, _ := setup(t)
	p, cards := sampleForOverview(t, s)
	prerequisite, dependent := cards[0], cards[1]
	save := func(id string, body string) boardCard {
		t.Helper()
		got := request(t, s, "POST", "/v1/board/"+id, body)
		var updated boardCard
		if got.Code != 200 || json.Unmarshal(got.Body.Bytes(), &updated) != nil {
			t.Fatal(got.Code, got.Body)
		}
		return updated
	}
	dependent = save(dependent.ID, fmt.Sprintf(`{"project_id":%q,"revision":%d,"stage":"planned","require_dependencies":true,"dependencies":[%q]}`,
		p.ID, dependent.Revision, prerequisite.ID))
	path := "/v1/board/" + dependent.ID + "/start"
	body := fmt.Sprintf(`{"project_id":%q,"revision":%d}`, p.ID, dependent.Revision)
	if got := request(t, s, "POST", path, body); got.Code != 409 || !strings.Contains(got.Body.String(), "WORK_DEPENDENCIES_PENDING") {
		t.Fatal(got.Code, got.Body)
	}
	workers, err := s.Store.Workspaces(t.Context())
	if err != nil || len(workers) != 0 {
		t.Fatal("blocked start allocated a worker", workers, err)
	}
	save(prerequisite.ID, fmt.Sprintf(`{"project_id":%q,"revision":%d,"stage":"done"}`, p.ID, prerequisite.Revision))
	worker := domainWorkspace(t, s, "control-token", p.ID, "dependency-worker")
	body = fmt.Sprintf(`{"project_id":%q,"revision":%d,"workspace_id":%q}`, p.ID, dependent.Revision, worker.ID)
	if got := request(t, s, "POST", path, body); got.Code != 202 {
		t.Fatal(got.Code, got.Body)
	}
}
