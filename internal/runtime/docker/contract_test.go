package docker

import (
	"agent-workbench/internal/runtime/controllerclient"
	"agent-workbench/internal/runtime/internal/contracttest"
	"agent-workbench/internal/runtime/protocol"
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestCapabilitiesThroughClient(t *testing.T) {
	c, _ := controller(t, false)
	server := httptest.NewServer(c.Handler())
	defer server.Close()
	caps, err := controllerclient.NewClient(server.URL, c.cfg.Token).Capabilities(context.Background())
	if err != nil || caps.ID != protocol.DevelopmentTemplate || caps.SchemaVersion != 1 || caps.Backend != "docker" || !slices.Equal(caps.NetworkModes, []string{"blocked", "allowlist", "open"}) || !caps.BranchSeed {
		t.Fatalf("capabilities contract: %+v, %v", caps, err)
	}
}
func TestHistoryThroughControllerAndWorkspace(t *testing.T) {
	c, engine := controller(t, false)
	contracttest.History(t, c.Handler(), c.cfg.Token, func() string {
		return c.token("ws_test", engine.spec["Labels"].(map[string]any)["workbench.generation"].(string))
	}, func(client *http.Client) { c.agentHTTP = client })
}

func TestControllerHTTPContract(t *testing.T) {
	c, _ := controller(t, false)
	contracttest.Controller(t, c.Handler(), c.cfg.Token, c.cfg.GatewayServiceToken, 405, 2048, 200)
	contracttest.HTTP(t, c.Handler(),
		contracttest.HTTPCase{Name: "empty start", Method: "POST", Path: "/v1/workspaces/ws_test/start", Token: c.cfg.Token, Body: "", Status: 400, Code: "UNSUPPORTED_WORKSPACE_SPEC"},
		contracttest.HTTPCase{Name: "agent limit", Method: "POST", Path: "/v1/workspaces/ws_test/agent/v1/tasks", Token: c.cfg.Token, Body: strings.Repeat(" ", (70<<10)+1), Status: 400, Code: "INVALID_REQUEST"},
		contracttest.HTTPCase{Name: "invalid network target", Method: "POST", Path: "/v1/workspaces/invalid.id/network-policy", Token: c.cfg.Token, Body: "{}", Status: 503, Code: "NETWORK_POLICY_UNAVAILABLE"},
	)
}
