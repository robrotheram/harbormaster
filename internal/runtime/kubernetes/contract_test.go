package kubernetes

import (
	"agent-workbench/internal/lifecycle"
	"agent-workbench/internal/runtime/controllerclient"
	"agent-workbench/internal/runtime/internal/contracttest"
	"agent-workbench/internal/runtime/protocol"
	"agent-workbench/internal/shared"
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestCapabilitiesThroughClient(t *testing.T) {
	k, _ := kubeProvider(t)
	server := httptest.NewServer(k.Handler())
	defer server.Close()
	caps, err := controllerclient.NewClient(server.URL, k.cfg.Token).Capabilities(context.Background())
	if err != nil || caps.ID != protocol.DevelopmentTemplate || caps.SchemaVersion != 1 || caps.Backend != "kubernetes-agent-sandbox" || caps.NetworkModes == nil || len(caps.NetworkModes) != 2 || caps.BranchSeed {
		t.Fatalf("capabilities contract: %+v, %v", caps, err)
	}
}
func TestHistoryThroughControllerAndWorkspace(t *testing.T) {
	k, fake := kubeProvider(t)
	contracttest.History(t, k.Handler(), k.cfg.Token, func() string {
		labels := fake.resources["project-one/test-ws-test"]["metadata"].(map[string]any)["labels"].(map[string]any)
		return k.token("ws_test", labels["workbench.generation"].(string))
	}, func(client *http.Client) { k.agentHTTP = client })
}
func TestKubernetesRefusesUnsupportedNetworkingBeforeAllocation(t *testing.T) {
	k, fake := kubeProvider(t)
	server := httptest.NewServer(k.Handler())
	defer server.Close()
	client := controllerclient.NewClient(server.URL, k.cfg.Token)
	for _, mode := range []string{"open"} {
		_, err := client.StartConfigured(context.Background(), "ws_test", protocol.WorkspaceSpec{ProjectID: "proj_one", HomeID: "home_one", AgentKind: "codex", ConnectionMode: mode})
		if err == nil || fake.creates != 0 {
			t.Fatal("unsupported network policy allocated a sandbox")
		}
	}
	if err := client.ConfigureConnections(context.Background(), "ws_test", shared.ConnectionPolicy{Mode: "blocked"}); err == nil || !strings.Contains(err.Error(), "NETWORK_POLICY_UNAVAILABLE") {
		t.Fatal("claimed blocked policy without an enforcement verifier", err)
	}
}

func TestNetworkPolicyThroughRealClient(t *testing.T) {
	k, _ := kubeProvider(t)
	server := httptest.NewServer(k.Handler())
	defer server.Close()
	client := controllerclient.NewClient(server.URL, k.cfg.Token)
	ctx := context.Background()
	if _, err := client.StartConfigured(ctx, "ws_test", protocol.WorkspaceSpec{ProjectID: "proj_one", HomeID: "home_one", AgentKind: "codex", ConnectionMode: "blocked"}); err != nil {
		t.Fatal(err)
	}
	if err := client.ConfigureConnections(ctx, "ws_test", shared.ConnectionPolicy{Mode: "blocked"}); err != nil {
		t.Fatal(err)
	}
	if err := client.ConfigureConnections(ctx, "ws_test", shared.ConnectionPolicy{Mode: "allowlist", Destinations: []string{"github.com:443"}}); err == nil {
		t.Fatal("project ceiling bypassed through client")
	}
	if err := client.Stop(ctx, "ws_test"); err != nil {
		t.Fatal(err)
	}
}

func TestRunningKubernetesWorkspaceCustomAllowlist(t *testing.T) {
	k, _ := kubeProvider(t)
	const id = "ws_test"
	const allowed = "registry.example.test:443"
	const denied = "outside.example.test:443"
	k.cfg.ProjectAllowlists = map[string][]string{"proj_one": {allowed}}
	server := httptest.NewServer(k.Handler())
	defer server.Close()
	client := controllerclient.NewClient(server.URL, k.cfg.Token)
	ctx := context.Background()
	if _, err := client.StartConfigured(ctx, id, protocol.WorkspaceSpec{
		ProjectID: "proj_one", HomeID: "home_one", AgentKind: "codex", ConnectionMode: "blocked",
	}); err != nil {
		t.Fatal(err)
	}

	o, err := client.Inspect(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	ws := shared.Workspace{ID: id}
	lifecycle.ApplyObservation(&ws, o, time.Now(), time.Hour)
	if ws.State != "READY" || ws.RuntimeType != "kata" || ws.ConnectionMode == "open" {
		t.Fatalf("expected a running, restricted Kubernetes workspace: %+v", ws)
	}
	policy := shared.ConnectionPolicy{Mode: "allowlist", Destinations: []string{allowed}}
	ws.Connections = &policy
	if err := lifecycle.CheckConnections(ws); err != nil {
		t.Fatalf("custom allowlist requires stopping the workspace: %v", err)
	}
	if err := client.ConfigureConnections(ctx, id, policy); err != nil {
		t.Fatalf("apply custom allowlist live: %v", err)
	}
	outsideProject := shared.ConnectionPolicy{Mode: "allowlist", Destinations: []string{allowed, denied}}
	if err := client.ConfigureConnections(ctx, id, outsideProject); err == nil || err.Error() != "NETWORK_POLICY_UNAVAILABLE" {
		t.Fatalf("expected controller to reject destination outside project allowlist, got %v", err)
	}

	resource, err := k.find(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	applied, err := k.readPolicy(ctx, resource)
	if err != nil || applied.Mode != policy.Mode || !slices.Equal(applied.Destinations, policy.Destinations) {
		t.Fatalf("applied policy = %+v, %v, want %+v", applied, err, policy)
	}
	o, err = client.Inspect(ctx, id)
	if err != nil || !o.Running || !o.Ready {
		t.Fatalf("workspace after policy update = %+v, %v", o, err)
	}
}

func TestControllerHTTPContract(t *testing.T) {
	k, _ := kubeProvider(t)
	contracttest.Controller(t, k.Handler(), k.cfg.Token, k.cfg.GatewayServiceToken, 405, 2048, 409)
	contracttest.HTTP(t, k.Handler(),
		contracttest.HTTPCase{Name: "empty start", Method: "POST", Path: "/v1/workspaces/ws_test/start", Token: k.cfg.Token, Body: "", Status: 400, Code: "INVALID_REQUEST"},
		contracttest.HTTPCase{Name: "agent limit", Method: "POST", Path: "/v1/workspaces/ws_test/agent/v1/tasks", Token: k.cfg.Token, Body: strings.Repeat(" ", (70<<10)+1), Status: 400, Code: "INVALID_REQUEST"},
		contracttest.HTTPCase{Name: "invalid network target", Method: "POST", Path: "/v1/workspaces/invalid.id/network-policy", Token: k.cfg.Token, Body: "{}", Status: 400, Code: "INVALID_REQUEST"},
	)
}
