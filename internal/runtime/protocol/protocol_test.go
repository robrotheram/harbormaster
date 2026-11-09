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
