package kubernetes

import (
	"agent-workbench/internal/runtime/controllerclient"
	"agent-workbench/internal/runtime/internal/contracttest"
	"agent-workbench/internal/runtime/protocol"
	"agent-workbench/internal/shared"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

func TestControllerHTTPContract(t *testing.T) {
	k, _ := kubeProvider(t)
	contracttest.Controller(t, k.Handler(), k.cfg.Token, k.cfg.GatewayServiceToken, 405, 2048, 409)
	contracttest.HTTP(t, k.Handler(),
		contracttest.HTTPCase{Name: "empty start", Method: "POST", Path: "/v1/workspaces/ws_test/start", Token: k.cfg.Token, Body: "", Status: 400, Code: "INVALID_REQUEST"},
		contracttest.HTTPCase{Name: "agent limit", Method: "POST", Path: "/v1/workspaces/ws_test/agent/v1/tasks", Token: k.cfg.Token, Body: strings.Repeat(" ", (70<<10)+1), Status: 400, Code: "INVALID_REQUEST"},
		contracttest.HTTPCase{Name: "invalid network target", Method: "POST", Path: "/v1/workspaces/invalid.id/network-policy", Token: k.cfg.Token, Body: "{}", Status: 400, Code: "INVALID_REQUEST"},
	)
}
