package dashboard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
)

type workerSessionRevoker interface {
	revokeWorkerSessions(string)
}

type interactiveSessions struct {
	http.Handler
	revoke func(string)
}

func (h interactiveSessions) revokeWorkerSessions(user string) { h.revoke(user) }

func revokeWorkerSessions(handler http.Handler, issuer, identity string) {
	if revoker, ok := handler.(workerSessionRevoker); ok && identity != "" {
		revoker.revokeWorkerSessions(subscriptionIdentity(issuer, identity))
	}
}

func subscriptionIdentity(issuer, id string) string {
	digest := sha256.Sum256([]byte(issuer + "\x00" + id))
	return "user_" + hex.EncodeToString(digest[:])
}

func withWorkerSession(r *http.Request, session context.Context) (*http.Request, func()) {
	ctx, cancel := context.WithCancel(r.Context())
	if session.Err() != nil {
		cancel()
	}
	stop := context.AfterFunc(session, cancel)
	return r.WithContext(ctx), func() { stop(); cancel() }
}
