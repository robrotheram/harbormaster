package dashboard

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
)

// NewLocalAuthHandler protects the operator origin even when Compose forwards
// loopback connections through a bridge. Worker and preview origins use tickets.
func NewLocalAuthHandler(next http.Handler, password string) (http.Handler, error) {
	if len(password) < 32 || strings.ContainsAny(password, " \t\r\n") {
		return nil, errors.New("local dashboard requires DASHBOARD_LOCAL_PASSWORD")
	}
	expected := sha256.Sum256([]byte(password))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		switch requestHostname(r) {
		case "localhost", "127.0.0.1":
			user, password, valid := r.BasicAuth()
			got := sha256.Sum256([]byte(password))
			if !valid || user != "operator" || subtle.ConstantTimeCompare(got[:], expected[:]) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="Harbormaster local", charset="UTF-8"`)
				w.Header().Set("Cache-Control", "no-store")
				writeError(w, 401, "AUTH_REQUIRED")
				return
			}
			r.Header.Del("Authorization")
		default:
			if !localWorkerHost(r) {
				writeError(w, 403, "HOST_REJECTED")
				return
			}
		}
		next.ServeHTTP(w, r)
	}), nil
}
