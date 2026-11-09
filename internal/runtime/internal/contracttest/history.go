// Package contracttest exercises a real controller client and guest handler.
package contracttest

import (
	"agent-workbench/internal/runtime/controllerclient"
	"agent-workbench/internal/runtime/protocol"
	"agent-workbench/internal/shared"
	"agent-workbench/internal/workspace"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func History(t *testing.T, handler http.Handler, token string, grant func() string, bindGuest func(*http.Client)) {
	t.Helper()
	ctx := context.Background()
	server := httptest.NewServer(handler)
	defer server.Close()
	client := controllerclient.NewClient(server.URL, token)
	spec := protocol.WorkspaceSpec{ProjectID: "proj_one", HomeID: "home_one", AgentKind: "codex"}
	if _, err := client.StartConfigured(ctx, "ws_test", spec); err != nil {
		t.Fatal(err)
	}
	guest, err := workspace.New(workspace.Config{ID: "ws_test", ProjectID: spec.ProjectID, HomeID: spec.HomeID, AgentKind: spec.AgentKind, Token: grant(), DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer guest.Close()
	bindGuest(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		w := httptest.NewRecorder()
		guest.Handler().ServeHTTP(w, r)
		return w.Result(), nil
	})})
	for _, query := range []string{"history=1&limit=20", "history=1&after=0&until=0&order=desc&limit=20", "history=1&episode=episode_one&before=0&limit=20"} {
		status, body, err := client.Agent(ctx, "ws_test", "GET", "/v1/tasks?"+query, nil)
		var page shared.HistoryPage
		if err != nil || status != 200 || json.Unmarshal(body, &page) != nil || page.SchemaVersion != 1 {
			t.Fatalf("history %s: status %d, %s, %v", query, status, body, err)
		}
	}
	for _, query := range []string{"history=1&url=http://elsewhere", "history=1&limit=20&limit=30", "history=1&after=-1"} {
		status, _, err := client.Agent(ctx, "ws_test", "GET", "/v1/tasks?"+query, nil)
		if err != nil || status != 400 {
			t.Fatalf("unbounded history query accepted: %s, %d, %v", query, status, err)
		}
	}
}
