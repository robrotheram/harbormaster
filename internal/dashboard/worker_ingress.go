package dashboard

import (
	"context"
	"net/http"
	"regexp"
	"strings"
)

type publicWorkerIngressKey struct{}

var previewDNSLabel = regexp.MustCompile(`^preview-[a-f0-9]{32}$`)

var workerDNSLabel = regexp.MustCompile(`^(desktop-)?ws-[a-z0-9][a-z0-9-]{0,60}$`)

// Hosted workers have separate origins and capability authentication. They must
// never receive the operator console's cookie or authenticated user authority.
func publicWorkerLabel(host, dashboardHost string) string {
	suffix := "." + dashboardHost
	if !strings.HasSuffix(host, suffix) {
		return ""
	}
	label := strings.TrimSuffix(host, suffix)
	if !workerDNSLabel.MatchString(label) && !previewDNSLabel.MatchString(label) {
		return ""
	}
	return label
}
func workerSecure(r *http.Request) bool {
	_, ok := r.Context().Value(publicWorkerIngressKey{}).(string)
	return ok
}

func servePublicWorker(next http.Handler, w http.ResponseWriter, r *http.Request, dashboardHost string) bool {
	label := publicWorkerLabel(strings.ToLower(r.Host), strings.ToLower(dashboardHost))
	if label == "" {
		return false
	}
	workerOrigin := "https://" + r.Host
	incoming := r.Header.Get("Origin")
	native := strings.HasPrefix(label, "desktop-") && (incoming == "t3code://app" || incoming == "t3code-dev://app")
	if incoming != "" && incoming != workerOrigin && !native {
		writeError(w, 403, "ORIGIN_REJECTED")
		return true
	}
	ctx := context.WithValue(r.Context(), publicWorkerIngressKey{}, label)
	ctx = context.WithValue(ctx, authenticatedOriginKey{}, workerOrigin)
	next.ServeHTTP(w, r.WithContext(ctx))
	return true
}
