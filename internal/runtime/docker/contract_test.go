package docker

import (
	"agent-workbench/internal/runtime/controllerclient"
	"agent-workbench/internal/runtime/internal/contracttest"
	"agent-workbench/internal/runtime/protocol"
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
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
