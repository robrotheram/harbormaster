package protocol

import "testing"

func TestRetainedServiceRoutesAndBudgets(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		want         bool
	}{
		{"POST", "/v1/services", true}, {"GET", "/v1/services/service_one", true}, {"POST", "/v1/services/service_one/fetch", true}, {"POST", "/v1/services/service_one/revoke", true}, {"POST", "/v1/services/reconcile", true}, {"GET", "/v1/services/service_one/fetch", false}, {"POST", "/v1/services/service_one/proxy", false}, {"POST", "/v1/services/../fetch", false},
	} {
		if AllowedAgentPath(tc.method, tc.path) != tc.want {
			t.Fatal(tc)
		}
	}
	if AgentBodyLimit("/v1/services") != 24<<20 || AgentResponseLimit("/v1/workspaces/ws_one/agent/v1/services/service_one/fetch") != 3<<20 {
		t.Fatal("service budgets missing")
	}
	if AgentBodyLimit("/v1/tasks") != 70<<10 || AgentResponseLimit("/v1/tasks/task_one") != 2<<20 {
		t.Fatal("metadata budgets widened")
	}
}
