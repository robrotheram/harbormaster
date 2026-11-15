package dashboard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
)

type subscriptionIdentityKey struct{}

func withSubscriptionIdentity(r *http.Request, issuer, id string) *http.Request {
	digest := sha256.Sum256([]byte(issuer + "\x00" + id))
	return r.WithContext(context.WithValue(r.Context(), subscriptionIdentityKey{}, "user_"+hex.EncodeToString(digest[:])))
}
