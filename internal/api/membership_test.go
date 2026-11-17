package api

import (
	"agent-workbench/internal/shared"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func humanRequest(t *testing.T, s *Server, email, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	id := "user_" + strings.NewReplacer("@", "_", ".", "_").Replace(email)
	token, err := shared.IssueHumanAccess(s.cfg.Token, shared.HumanIdentity{ID: id, Email: email}, s.now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	return domainRequest(t, s, token, method, path, body)
}
func TestExplicitMembershipInvitationAcceptanceAndRevocation(t *testing.T) {
	s, _ := setup(t)
	project := domainProject(t, s, "members")
	path := "/v1/projects/" + project.ID
	// Signing in alone grants neither discovery nor direct project access.
	if got := humanRequest(t, s, "alice@example.com", "GET", "/v1/projects", ""); got.Code != 200 || strings.Contains(got.Body.String(), project.ID) {
		t.Fatal(got.Code, got.Body)
	}
	if got := humanRequest(t, s, "alice@example.com", "GET", "/v1/work-items?project_id="+project.ID, ""); got.Code != 404 {
		t.Fatal(got.Code, got.Body)
	}
	invite := `{"email":"alice@example.com","role":"admin","revision":0,"action":"invite"}`
	if got := request(t, s, "POST", path+"/members", invite); got.Code != 200 {
		t.Fatal(got.Code, got.Body)
	}
	if got := humanRequest(t, s, "bob@example.com", "POST", path+"/accept-invitation", `{"revision":1}`); got.Code != 409 {
		t.Fatal("foreign invite", got.Code, got.Body)
	}
	if got := humanRequest(t, s, "alice@example.com", "GET", "/v1/project-invitations", ""); got.Code != 200 || !strings.Contains(got.Body.String(), project.ID) {
		t.Fatal(got.Code, got.Body)
	}
	if got := humanRequest(t, s, "alice@example.com", "POST", path+"/accept-invitation", `{"revision":1}`); got.Code != 200 {
		t.Fatal(got.Code, got.Body)
	}
	if got := humanRequest(t, s, "alice@example.com", "GET", "/v1/projects", ""); got.Code != 200 || !strings.Contains(got.Body.String(), project.ID) {
		t.Fatal(got.Code, got.Body)
	}
	if got := humanRequest(t, s, "alice@example.com", "POST", path+"/members", `{"email":"alice@example.com","role":"admin","revision":2,"action":"revoke"}`); got.Code != 409 {
		t.Fatal("last admin", got.Code, got.Body)
	}
	if got := humanRequest(t, s, "alice@example.com", "POST", path+"/members", `{"email":"bob@example.com","role":"member","revision":0,"action":"invite"}`); got.Code != 200 {
		t.Fatal(got.Code, got.Body)
	}
	if got := humanRequest(t, s, "bob@example.com", "POST", path+"/accept-invitation", `{"revision":1}`); got.Code != 200 {
		t.Fatal(got.Code, got.Body)
	}
	if got := humanRequest(t, s, "bob@example.com", "POST", path+"/members", invite); got.Code != 403 {
		t.Fatal("member managed roster", got.Code)
	}
	if got := humanRequest(t, s, "alice@example.com", "POST", path+"/members", `{"email":"bob@example.com","role":"member","revision":2,"action":"revoke"}`); got.Code != 200 {
		t.Fatal(got.Code, got.Body)
	}
	if got := humanRequest(t, s, "bob@example.com", "GET", path+"/members", ""); got.Code != 404 {
		t.Fatal("revoked access", got.Code, got.Body)
	}
	if got := humanRequest(t, s, "bob@example.com", "POST", path+"/accept-invitation", `{"revision":1}`); got.Code != 409 {
		t.Fatal("replay revived member", got.Code)
	}
}
func TestHumanProjectCreationIsOwnedAndRetriesAreIdentityScoped(t *testing.T) {
	s, _ := setup(t)
	body := `{"name":"Private project","idempotency_key":"same-key"}`
	ids := map[string]string{}
	for _, email := range []string{"alice@example.com", "bob@example.com"} {
		got := humanRequest(t, s, email, "POST", "/v1/projects", body)
		if got.Code != 201 {
			t.Fatal(got.Code, got.Body)
		}
		var p shared.Project
		json.Unmarshal(got.Body.Bytes(), &p)
		ids[email] = p.ID
		got = humanRequest(t, s, email, "GET", "/v1/projects/"+p.ID+"/members", "")
		if got.Code != 200 || !strings.Contains(got.Body.String(), `"can_manage":true`) {
			t.Fatal(got.Code, got.Body)
		}
		got = humanRequest(t, s, email, "POST", "/v1/projects", body)
		if got.Code != 200 {
			t.Fatal(got.Code, got.Body)
		}
	}
	if ids["alice@example.com"] == ids["bob@example.com"] {
		t.Fatal("cross-user retry collision")
	}
	got := humanRequest(t, s, "bob@example.com", "POST", "/v1/board", fmt.Sprintf(`{"project_id":%q,"title":"Unauthorized","kind":"software","idempotency_key":"create"}`, ids["alice@example.com"]))
	if got.Code != 404 {
		t.Fatal(got.Code, got.Body)
	}
}

func TestMembersCannotExecuteInCrossProjectSharedWorkerMode(t *testing.T) {
	s, _ := setup(t)
	s.cfg.SharedWorkspaceID = "ws_shared"
	for _, path := range []string{"/v1/workspaces/ws_shared/start", "/v1/work-items/work_one/continue", "/v1/board/work_one/start", "/v1/projects/default/hermes/assistant"} {
		got := humanRequest(t, s, "alice@example.com", "POST", path, `{}`)
		if got.Code != 409 || !strings.Contains(got.Body.String(), "PROJECT_ISOLATED_WORKERS_REQUIRED") {
			t.Fatal(path, got.Code, got.Body)
		}
	}
}

func TestControllerHandoffAndAssignedReviewerAreEnforced(t *testing.T) {
	s, _ := setup(t)
	work := boardWork(t, s)
	root := "/v1/work-items/" + work.ID
	for _, email := range []string{"alice@example.com", "bob@example.com"} {
		got := request(t, s, "POST", "/v1/projects/default/members", fmt.Sprintf(`{"email":%q,"role":"member","revision":0,"action":"invite"}`, email))
		if got.Code != 200 {
			t.Fatal(got.Code, got.Body)
		}
		got = humanRequest(t, s, email, "POST", "/v1/projects/default/accept-invitation", `{"revision":1}`)
		if got.Code != 200 {
			t.Fatal(got.Code, got.Body)
		}
	}
	claim := `{"project_id":"default","controller_id":"user_alice_example_com","reviewer_id":"user_alice_example_com","expected_controller_id":"","expected_reviewer_id":"","expected_attempt_id":""}`
	if got := humanRequest(t, s, "alice@example.com", "POST", root+"/responsibility", claim); got.Code != 200 {
		t.Fatal(got.Code, got.Body)
	}
	if got := humanRequest(t, s, "bob@example.com", "POST", root+"/run", `{"project_id":"default","idempotency_key":"forbidden"}`); got.Code != 403 {
		t.Fatal("noncontroller ran", got.Code, got.Body)
	}
	handoff := `{"project_id":"default","controller_id":"user_bob_example_com","reviewer_id":"user_alice_example_com","expected_controller_id":"user_alice_example_com","expected_reviewer_id":"user_alice_example_com","expected_attempt_id":""}`
	if got := humanRequest(t, s, "alice@example.com", "POST", root+"/responsibility", handoff); got.Code != 200 {
		t.Fatal(got.Code, got.Body)
	}
	if got := humanRequest(t, s, "alice@example.com", "POST", root+"/run", `{"project_id":"default","idempotency_key":"old-controller"}`); got.Code != 403 {
		t.Fatal(got.Code, got.Body)
	}
	got := humanRequest(t, s, "bob@example.com", "POST", root+"/run", `{"project_id":"default","idempotency_key":"new-controller"}`)
	if got.Code != 202 {
		t.Fatal(got.Code, got.Body)
	}
	var attempt shared.WorkAttempt
	json.Unmarshal(got.Body.Bytes(), &attempt)
	// Simulate recorded worker completion without involving a live runtime.
	itemResponse := request(t, s, "GET", root+"?project_id=default", "")
	json.Unmarshal(itemResponse.Body.Bytes(), &work)
	work.State = "awaiting_review"
	attempt.State = "awaiting_review"
	raw, _ := json.Marshal(work)
	if _, err := s.Store.DB.Exec(`UPDATE work_items SET metadata=$1 WHERE id=$2`, string(raw), work.ID); err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(attempt)
	if _, err := s.Store.DB.Exec(`UPDATE work_attempts SET metadata=$1,active_workspace=NULL WHERE id=$2`, string(raw), attempt.ID); err != nil {
		t.Fatal(err)
	}
	decision := fmt.Sprintf(`{"project_id":"default","accepted":true,"expected_attempt_id":%q}`, attempt.ID)
	if got := humanRequest(t, s, "bob@example.com", "POST", root+"/review", decision); got.Code != 403 {
		t.Fatal("wrong reviewer", got.Code, got.Body)
	}
	if got := humanRequest(t, s, "alice@example.com", "POST", root+"/review", decision); got.Code != 200 {
		t.Fatal(got.Code, got.Body)
	}
	got = request(t, s, "GET", root+"/attempts?project_id=default", "")
	if !strings.Contains(got.Body.String(), `"reviewed_by":"user_alice_example_com"`) {
		t.Fatal("missing review attribution", got.Body)
	}
}
