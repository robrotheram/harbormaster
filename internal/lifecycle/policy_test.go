package lifecycle

import (
	"agent-workbench/internal/shared"
	"testing"
	"time"
)

func TestUnknownAndBusyActivityCannotProduceIdleDeadline(t *testing.T) {
	now := time.Now().UTC()
	for _, activity := range []shared.Activity{{Known: false, LastActivity: now}, {Known: true, ActiveRuns: 1, LastActivity: now}, {Known: true, BackgroundJobs: 1, LastActivity: now}} {
		w := shared.Workspace{State: "READY", IdleStopAt: &now}
		ApplyObservation(&w, shared.Observation{Running: true, Ready: true, Activity: activity}, now, time.Hour)
		if w.IdleStopAt != nil {
			t.Fatal("unsafe idle deadline", w)
		}
	}
}
func TestConnectionTransitionsShareOnePolicy(t *testing.T) {
	tests := []struct {
		runtime, state, observed, desired string
		version                           int
		want                              string
	}{
		{"firecracker", "STOPPED", "", "open", 2, "OPEN_NETWORK_FORBIDDEN_IN_SANDBOX"},
		{"docker", "READY", "blocked", "open", 2, "STOP_WORKSPACE_FIRST"},
		{"docker", "STOPPED", "blocked", "open", 2, ""},
		{"docker", "READY", "allowlist", "allowlist", 1, "STOP_WORKSPACE_FIRST"},
		{"docker", "READY", "allowlist", "allowlist", 2, ""},
	}
	for _, tt := range tests {
		w := shared.Workspace{RuntimeType: tt.runtime, State: tt.state, ConnectionMode: tt.observed, ConnectionProxyVersion: tt.version, Connections: &shared.ConnectionPolicy{Mode: tt.desired, Destinations: []string{"registry.npmjs.org:443"}}}
		err := CheckConnections(w)
		got := ""
		if err != nil {
			got = err.Error()
		}
		if got != tt.want {
			t.Fatalf("%+v: got %q", tt, got)
		}
	}
}
