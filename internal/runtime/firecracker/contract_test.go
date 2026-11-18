package firecracker

import (
	"agent-workbench/internal/runtime/controllerclient"
	"agent-workbench/internal/runtime/internal/contracttest"
	"agent-workbench/internal/runtime/protocol"
	"agent-workbench/internal/shared"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestCapabilitiesThroughClient(t *testing.T) {
	f := testFC()
	server := httptest.NewServer(f.Handler())
	defer server.Close()
	caps, err := controllerclient.NewClient(server.URL, f.token).Capabilities(context.Background())
	if err != nil || caps.ID != protocol.DevelopmentTemplate || caps.SchemaVersion != 1 || caps.Backend != "firecracker" || !slices.Equal(caps.NetworkModes, []string{"blocked", "allowlist"}) || caps.BranchSeed {
		t.Fatalf("capabilities contract: %+v, %v", caps, err)
	}
}
func TestFirecrackerFailedTerminationKeepsCapacityAndRevokesAuthority(t *testing.T) {
	f := testFC()
	v := &fcVM{ID: "ws_test", Generation: "generation_one", running: true, Policy: shared.ConnectionPolicy{Mode: "allowlist", Destinations: []string{"github.com:443"}}}
	f.vms[v.ID] = v
	f.terminateCgroup = func(string) error { return errors.New("termination not confirmed") }
	if err := f.kill(v); err == nil {
		t.Fatal("termination failure was ignored")
	}
	observed := f.observation(context.Background(), v)
	if !observed.Running || observed.Ready || observed.Activity.Known || !v.stopping || v.Policy.Mode != "blocked" {
		t.Fatalf("unconfirmed VM treated as stopped or ready: %+v", observed)
	}
	body := `{"token":"` + f.grant(v) + `","audience":"inference"}`
	if response := fcRequest(f, "POST", "/v1/grants/verify", f.service, body); response.Code != http.StatusForbidden {
		t.Fatal("stopping VM retained authority", response.Code)
	}
	f.terminateCgroup = func(string) error { return nil }
	if err := f.kill(v); err != nil || v.running || v.stopping {
		t.Fatal("confirmed absence did not release capacity", err)
	}
}

func TestFailedEnforcementCheckDoesNotClaimUnconfirmedTermination(t *testing.T) {
	f := testFC()
	v := &fcVM{ID: "ws_test", Generation: "missing-cgroup", running: true}
	f.vms[v.ID] = v
	f.terminateCgroup = func(string) error { return errors.New("host observation unavailable") }
	// The absent host cgroup makes verification fail; termination also fails.
	o := f.observation(context.Background(), v)
	if !o.Running || o.Ready || !v.stopping {
		t.Fatalf("failed verification fabricated a stopped VM: %+v", o)
	}
	// Retrying stop must finish host cleanup without depending on a dead guest.
	f.terminateCgroup = func(string) error { return nil }
	response := fcRequest(f, "POST", "/v1/workspaces/"+v.ID+"/stop", f.token, "")
	if response.Code != http.StatusOK || v.running {
		t.Fatal("could not retry termination after guest became unavailable", response.Code)
	}
}

func TestControllerHTTPContract(t *testing.T) {
	f := testFC()
	contracttest.Controller(t, f.Handler(), f.token, f.service, 404, 32768, 200)
	contracttest.HTTP(t, f.Handler(),
		contracttest.HTTPCase{Name: "invalid start target", Method: "POST", Path: "/v1/workspaces/invalid.id/start", Token: f.token, Body: "{}", Status: 400, Code: "INVALID_REQUEST"},
		contracttest.HTTPCase{Name: "absent guest checked before agent body", Method: "POST", Path: "/v1/workspaces/ws_test/agent/v1/tasks", Token: f.token, Body: strings.Repeat(" ", (70<<10)+1), Status: 503, Code: "NOT_READY"},
		contracttest.HTTPCase{Name: "wrong method", Method: "GET", Path: "/v1/workspaces/ws_test/start", Token: f.token, Body: "", Status: 404, Code: "NOT_FOUND"},
		contracttest.HTTPCase{Name: "decoded action", Method: "GET", Path: "/v1/workspaces/ws_test/agent%2Fv1/tasks", Token: f.token, Body: "", Status: 503, Code: "NOT_READY"},
	)
}

type policyRequestBody struct {
	io.Reader
	beforeRead func()
}

func (b *policyRequestBody) Read(p []byte) (int, error) {
	if b.beforeRead != nil {
		b.beforeRead()
		b.beforeRead = nil
	}
	return b.Reader.Read(p)
}

func TestNetworkPolicyRequestStaysBoundToOriginalVM(t *testing.T) {
	f := testFC()
	original := &fcVM{ID: "ws_test", Generation: "original", running: true}
	replacement := &fcVM{ID: original.ID, Generation: "replacement", running: true, Policy: shared.ConnectionPolicy{Mode: "blocked"}}
	f.vms[original.ID] = original
	body := &policyRequestBody{
		Reader: strings.NewReader(`{"connections":{"mode":"allowlist","destinations":["github.com:443"]}}`),
		beforeRead: func() {
			f.mu.Lock()
			defer f.mu.Unlock()
			original.running = false
			f.vms[original.ID] = replacement
		},
	}
	r := httptest.NewRequest("POST", "/v1/workspaces/ws_test/network-policy", body)
	r.Header.Set("Authorization", "Bearer "+f.token)
	response := httptest.NewRecorder()
	f.Handler().ServeHTTP(response, r)
	if response.Code != http.StatusConflict || replacement.Policy.Mode != "blocked" {
		t.Fatalf("policy crossed generations: response %d %s, replacement policy %+v", response.Code, response.Body, replacement.Policy)
	}
}
