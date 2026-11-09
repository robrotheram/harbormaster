package controllerclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
