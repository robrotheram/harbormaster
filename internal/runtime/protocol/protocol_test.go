package protocol

import "testing"

func TestQueriesAreScopedToTheirWorkspaceOperation(t *testing.T) {
	for _, tt := range []struct {
		method, path, query string
		valid               bool
	}{
		{"GET", "/v1/tasks", "history=1&limit=20&after=0&until=10&order=desc", true},
		{"GET", "/v1/tasks/task_one/result", "offset=3&limit=65536", true},
		{"GET", "/v1/tasks", "history=1&limit=20&limit=30", false},
		{"GET", "/v1/tasks", "history=1;target=elsewhere", false},
		{"GET", "/v1/tasks", "episode=../other", false},
		{"GET", "/v1/tasks/task_one/result", "history=1", false},
		{"POST", "/v1/tasks/task_one/cancel", "offset=0", false},
		{"GET", "/v1/activity", "history=1", false},
	} {
		_, err := AgentQuery(tt.method, tt.path, tt.query)
		if (err == nil) != tt.valid {
			t.Errorf("%s %s?%s: %v", tt.method, tt.path, tt.query, err)
		}
	}
}

func TestFactoryAgentPathsAndResponseBounds(t *testing.T) {
	for _, tt := range []struct {
		method, path string
		valid        bool
	}{
		{"POST", "/v1/artifacts", true}, {"GET", "/v1/artifacts", true},
		{"GET", "/v1/artifacts/artifact_one/content", true}, {"POST", "/v1/artifacts/artifact_one/content", false},
		{"GET", "/v1/artifacts/../content", false}, {"GET", "/v1/artifacts/artifact_one/content/more", false},
		{"POST", "/v1/recipe-jobs", true}, {"GET", "/v1/recipe-jobs/job_one", true},
		{"POST", "/v1/recipe-jobs/job_one/cancel", true}, {"GET", "/v1/recipe-jobs/job_one/cancel", false},
		{"POST", "/v1/recipe-jobs/job_one/exec", false},
	} {
		if got := AllowedAgentPath(tt.method, tt.path); got != tt.valid {
			t.Errorf("%s %s: %v", tt.method, tt.path, got)
		}
	}
	for _, path := range []string{"/v1/artifacts/artifact_one/content", "/v1/workspaces/ws_one/agent/v1/artifacts/artifact_one/content"} {
		if AgentResponseLimit(path) != 32<<20 {
			t.Fatal("artifact limit", path)
		}
	}
	if AgentResponseLimit("/v1/tasks/task_one/result") != 2<<20 {
		t.Fatal("ordinary response budget widened")
	}
}

func TestTerminalRoutesAreExplicit(t *testing.T) {
	for _, tt := range []struct {
		method, path string
		valid        bool
	}{
		{"POST", "/v1/terminals", true}, {"POST", "/v1/terminals/terminal_one/exchange", true}, {"POST", "/v1/terminals/terminal_one/close", true},
		{"GET", "/v1/terminals", false}, {"POST", "/v1/terminals/terminal_one/exec", false}, {"POST", "/v1/terminals/../exchange", false}, {"POST", "/v1/terminals/terminal_one/exchange/more", false},
	} {
		if AllowedAgentPath(tt.method, tt.path) != tt.valid {
			t.Fatal(tt)
		}
	}
	if _, err := AgentQuery("POST", "/v1/terminals", "command=id"); err == nil {
		t.Fatal("terminal query accepted")
	}
}
