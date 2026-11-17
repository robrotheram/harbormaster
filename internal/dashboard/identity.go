package dashboard

import (
	"agent-workbench/internal/shared"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"
)

type subscriptionIdentityKey struct{}

func withSubscriptionIdentity(r *http.Request, issuer, id string) *http.Request {
	digest := sha256.Sum256([]byte(issuer + "\x00" + id))
	return r.WithContext(context.WithValue(r.Context(), subscriptionIdentityKey{}, "user_"+hex.EncodeToString(digest[:])))
}

// Only authentication middleware may establish a human principal. Browser
// headers cannot supply it; local operator mode deliberately retains recovery access.
type humanIdentityKey struct{}

func withHumanIdentity(r *http.Request, issuer, id, email string) *http.Request {
	r = withSubscriptionIdentity(r, issuer, id)
	identity, _ := r.Context().Value(subscriptionIdentityKey{}).(string)
	return r.WithContext(context.WithValue(r.Context(), humanIdentityKey{}, shared.HumanIdentity{ID: identity, Email: strings.ToLower(strings.TrimSpace(email))}))
}
func humanToken(r *http.Request, secret string) string {
	h, ok := r.Context().Value(humanIdentityKey{}).(shared.HumanIdentity)
	if !ok {
		return secret
	}
	token, err := shared.IssueHumanAccess(secret, h, time.Now().Add(2*time.Minute))
	if err != nil {
		return "invalid-human-identity"
	}
	return token
}
