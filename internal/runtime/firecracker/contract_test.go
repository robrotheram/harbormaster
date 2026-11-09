package firecracker

import (
	"agent-workbench/internal/runtime/controllerclient"
	"agent-workbench/internal/runtime/protocol"
	"agent-workbench/internal/shared"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
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
