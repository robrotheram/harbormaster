package firecracker

import (
	"agent-workbench/internal/runtime/protocol"
	"agent-workbench/internal/shared"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func matrixTestConfig() shared.HermesMatrix {
	return shared.HermesMatrix{Homeserver: "https://matrix.example.org", BotUserID: "@bot:example.org", RoomIDs: []string{"!approved:example.org"}, SenderIDs: []string{"@alice:example.org"}}
}

func TestMatrixProxyRoutesFenceRoomsAndMutations(t *testing.T) {
	c := matrixTestConfig()
	for _, tt := range []struct {
		method, path string
		allow        bool
	}{
		{"GET", "/_matrix/client/v3/sync", true},
		{"GET", "/_matrix/client/v3/rooms/!approved:example.org/state/m.room.encryption", true},
		{"PUT", "/_matrix/client/v3/rooms/!approved:example.org/send/m.room.encrypted/one", true},
		{"PUT", "/_matrix/client/v3/rooms/!approved:example.org/send/m.room.message/one", false},
		{"PUT", "/_matrix/client/v3/rooms/!foreign:example.org/send/m.room.encrypted/one", false},
		{"POST", "/_matrix/client/v3/join/!foreign:example.org", false},
		{"POST", "/_matrix/client/v3/rooms/!approved:example.org/invite", false},
		{"GET", "/_matrix/client/v3/rooms/!foreign:example.org/state", false},
		{"GET", "/_matrix/client/v3/rooms/!approved:example.org/state/../../!foreign:example.org/state", false},
		{"GET", "/_matrix/client/v3/rooms/!approved:example.org/messages", false},
		{"GET", "/_matrix/client/v3/rooms/!approved:example.org/event/sender-bypass", false},
		{"POST", "/_matrix/client/v3/login", false},
		{"GET", "/arbitrary-url", false},
	} {
		if got := matrixRouteAllowed(tt.method, tt.path, c); got != tt.allow {
			t.Fatal(tt.method, tt.path, got)
		}
	}
}

func TestMatrixSyncDropsForeignRoomsInvitesAndSendersBeforeGuest(t *testing.T) {
	body := []byte(`{"next_batch":"next","presence":{"events":[{"content":"PRESENCE_SENTINEL"}]},"rooms":{"invite":{"!invite:example.org":{"content":"INVITE_SENTINEL"}},"knock":{"!knock:example.org":{"content":"KNOCK_SENTINEL"}},"join":{"!foreign:example.org":{"timeline":{"events":[{"sender":"@alice:example.org","type":"m.room.encrypted","content":"FOREIGN_SENTINEL"}]}},"!approved:example.org":{"timeline":{"events":[{"sender":"@mallory:example.org","type":"m.room.encrypted","content":"UNAUTHORIZED_SENTINEL"},{"sender":"@mallory:example.org","type":"m.reaction","content":"REACTION_SENTINEL"},{"sender":"@alice:example.org","type":"m.room.encrypted","content":"APPROVED_MESSAGE"}]}}}}}`)
	out, err := filterMatrixSync(body, matrixTestConfig())
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"FOREIGN_SENTINEL", "INVITE_SENTINEL", "KNOCK_SENTINEL", "UNAUTHORIZED_SENTINEL", "REACTION_SENTINEL", "PRESENCE_SENTINEL"} {
		if strings.Contains(string(out), marker) {
			t.Fatal("project boundary leaked", marker)
		}
	}
	if !strings.Contains(string(out), "APPROVED_MESSAGE") || !strings.Contains(string(out), "next_batch") {
		t.Fatal("legitimate sync lost")
	}
}

func TestMatrixCredentialCannotReachPrivateHost(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer server.Close()
	c := matrixTestConfig()
	c.Homeserver = server.URL
	if validateMatrixCredential(context.Background(), c, "private-credential") == nil || calls != 0 {
		t.Fatal("private host reached", calls)
	}
}

func TestMatrixRouteCannotBeUsedBeforeExplicitActivation(t *testing.T) {
	f := testFC()
	v := &fcVM{ID: "one", Generation: "generation-one", running: true, Spec: protocol.WorkspaceSpec{ProjectID: "project", HermesMatrix: func() *shared.HermesMatrix { c := matrixTestConfig(); return &c }()}}
	r := httptest.NewRequest("GET", "http://matrix/_matrix/client/v3/sync", nil)
	r.Header.Set("Authorization", "Bearer "+f.grant(v))
	w := httptest.NewRecorder()
	f.matrixProxy(v).ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("workspace gained Matrix access before connection", w.Code)
	}
}
