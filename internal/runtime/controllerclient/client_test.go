package controllerclient

import (
	"agent-workbench/internal/runtime/protocol"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestAgentKindsSupportsLegacyFirecrackerWithoutRelaxingAdmission(t *testing.T) {
	legacy := `{"id":"development-v1","version":"firecracker-2","agent_kinds":["t3","codex","hermes"],"isolation_status":"jailed-vm","network_status":"no-nic-vsock","private_homes":true}`
	modern, _ := json.Marshal(protocol.RuntimeCapability{SchemaVersion: 1, ID: protocol.DevelopmentTemplate, Version: "firecracker-2", AgentKinds: []string{"t3", "codex", "hermes"}, NetworkModes: []string{"blocked", "allowlist"}, Resources: protocol.ResourceLimits{CPUCores: 1, MemoryBytes: 1 << 30}})
	for _, tt := range []struct {
		name, body string
		status     int
		validKinds bool
		validCaps  bool
	}{
		{"legacy", legacy, 200, true, false},
		{"modern", string(modern), 200, true, true},
		{"unauthorized", legacy, 401, false, false},
		{"unavailable", legacy, 503, false, false},
		{"malformed", `{`, 200, false, false},
		{"unknown schema", `{"schema_version":2,"id":"development-v1","version":"firecracker-2","agent_kinds":["hermes"]}`, 200, false, false},
		{"unknown legacy", `{"id":"development-v1","version":"unknown","agent_kinds":["hermes"]}`, 200, false, false},
		{"missing identity", `{"version":"firecracker-2","agent_kinds":["hermes"]}`, 200, false, false},
		{"missing kinds", `{"id":"development-v1","version":"firecracker-2"}`, 200, false, false},
		{"incomplete modern", `{"schema_version":1,"id":"development-v1","version":"firecracker-2","agent_kinds":["hermes"]}`, 200, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != "/v1/capabilities" || r.Header.Get("Authorization") != "Bearer management-token" {
					t.Error("capability query lost route or controller authority")
				}
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			}))
			defer server.Close()
			client := NewClient(server.URL, "management-token")
			kinds, err := client.AgentKinds(context.Background())
			if (err == nil) != tt.validKinds || tt.validKinds && !slices.Equal(kinds, []string{"t3", "codex", "hermes"}) {
				t.Fatalf("agent kinds: %v, %v", kinds, err)
			}
			if _, err := client.Capabilities(context.Background()); (err == nil) != tt.validCaps {
				t.Fatalf("workspace admission capability check: %v", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if _, err := client.AgentKinds(ctx); err == nil {
				t.Fatal("cancelled probe succeeded")
			}
		})
	}
}

func TestCapabilitiesRejectIncompleteOrUnsupportedWireContracts(t *testing.T) {
	for _, body := range []string{
		`{"template":{"id":"development-v1","version":"1","agent_kinds":["codex"]}}`,
		`{"schema_version":2,"id":"development-v1","version":"1","agent_kinds":["codex"],"network_modes":[]}`,
		`{"schema_version":1,"id":"development-v1","version":"1","agent_kinds":["codex"]}`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer management-token" {
				t.Error("capability query lost controller authority")
			}
			_, _ = io.WriteString(w, body)
		}))
		_, err := NewClient(server.URL, "management-token").Capabilities(context.Background())
		server.Close()
		if err == nil {
			t.Fatal("incomplete capability response was treated as supported", body)
		}
	}
}

func TestArtifactTransferHasSeparateResponseBudget(t *testing.T) {
	payload := make([]byte, 3<<20)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(payload) }))
	defer server.Close()
	client := NewClient(server.URL, "controller-token")
	status, body, err := client.Agent(context.Background(), "ws_one", "GET", "/v1/artifacts/artifact_one/content", nil)
	if err != nil || status != 200 || len(body) != len(payload) {
		t.Fatalf("artifact transfer %d %d %v", status, len(body), err)
	}
	if _, _, err = client.Agent(context.Background(), "ws_one", "GET", "/v1/tasks/task_one/result", nil); err == nil {
		t.Fatal("metadata response budget widened")
	}
}
