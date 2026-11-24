package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"agent-workbench/internal/shared"
)

func TestLocalDashboardBearerCannotIssueProjectCredentials(t *testing.T) {
	s, _ := setup(t)
	s.cfg.DashboardLocalOperatorToken = strings.Repeat("d", 32)
	if r := domainRequest(t, s, s.cfg.DashboardLocalOperatorToken, "GET", "/v1/projects", ""); r.Code != http.StatusOK {
		t.Fatalf("dashboard project list: %d %s", r.Code, r.Body)
	}
	if r := domainRequest(t, s, s.cfg.DashboardLocalOperatorToken, "POST", "/v1/projects/default/agent-access", "{}"); r.Code != http.StatusForbidden {
		t.Fatalf("dashboard bearer reached operator credential issuance: %d %s", r.Code, r.Body)
	}
}

func TestWorkerQuotaIsPerAuthenticatedUserAndPrivate(t *testing.T) {
	s, _ := setup(t)
	s.cfg.MaxRunningPerUser = 1
	project := func(email, key string) shared.Project {
		r := humanRequest(t, s, email, "POST", "/v1/projects", `{"name":"Quota project","idempotency_key":"`+key+`"}`)
		var p shared.Project
		if r.Code != http.StatusCreated || json.Unmarshal(r.Body.Bytes(), &p) != nil {
			t.Fatalf("create project: %d %s", r.Code, r.Body)
		}
		return p
	}
	alice, bob := "alice@example.com", "bob@example.com"
	a, b := project(alice, "quota-alice"), project(bob, "quota-bob")
	a1 := domainWorkspace(t, s, "control-token", a.ID, "quota-one")
	a2 := domainWorkspace(t, s, "control-token", a.ID, "quota-two")
	b1 := domainWorkspace(t, s, "control-token", b.ID, "quota-three")
	start := func(email, id string, want int) {
		t.Helper()
		r := humanRequest(t, s, email, "POST", "/v1/workspaces/"+id+"/start", `{"idempotency_key":"start-quota"}`)
		if r.Code != want || strings.Contains(r.Body.String(), "quota_user_id") {
			t.Fatalf("start worker: %d %s", r.Code, r.Body)
		}
	}
	start(alice, a1.ID, http.StatusOK)
	reserved, err := s.Store.Workspace(t.Context(), a1.ID)
	if err != nil || reserved.QuotaUserID == "" {
		t.Fatalf("quota attribution was not persisted: %v", err)
	}
	start(alice, a2.ID, http.StatusTooManyRequests)
	start(bob, b1.ID, http.StatusOK)
}
