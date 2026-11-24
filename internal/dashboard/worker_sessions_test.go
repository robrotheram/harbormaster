package dashboard

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"agent-workbench/internal/shared"
)

func TestPreviewLogoutCancelsActiveWorkerRequests(t *testing.T) {
	p := newPreviewIngress(nil, "", "")
	human := shared.HumanIdentity{ID: subscriptionIdentity("issuer", "person")}
	session, cancel := context.WithCancel(t.Context())
	defer cancel()
	binding := previewBinding{Human: &human, Host: "preview.localhost", Expiry: time.Now().Add(time.Hour), session: session, cancel: cancel}
	p.tickets["pending"] = previewBinding{Human: &human}
	p.sessions["existing"] = binding
	p.sessions["unrelated"] = previewBinding{Human: &shared.HumanIdentity{ID: "another-user"}}

	request, release := withWorkerSession(httptest.NewRequest("GET", "http://preview.localhost/", nil), session)
	defer release()
	revokeWorkerSessions(interactiveSessions{revoke: p.revoke}, "issuer", "person")
	select {
	case <-request.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("logout did not cancel the in-flight preview")
	}
	if len(p.tickets) != 0 || len(p.sessions) != 1 || p.sessions["unrelated"].Human == nil {
		t.Fatal("logout did not revoke only this person's preview bindings")
	}
}
