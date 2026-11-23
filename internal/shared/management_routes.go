package shared

import (
	"net/http"
	"strings"
)

type sharedHumanRule uint8

const (
	noSharedHuman sharedHumanRule = iota
	sharedHumanExact
	sharedHumanAnyExact
	sharedHumanTrimmed
	sharedHumanAnyTrimmed
	sharedHumanTrimmedID
	sharedHumanWorkItem
	sharedHumanAttempts
)

type managementRoute struct {
	method, pattern          string
	dashboardLimit, apiLimit int64
	sharedHuman              sharedHumanRule
}

// The two limits apply at different layers. Zero means the dashboard does
// not proxy the route or the API does not enforce a body limit at decode time.
// Patterns with API limits use the ServeMux parameter names.
var managementRoutes = []managementRoute{
	// Subscriptions are owner-scoped. Loading them on a worker still requires isolation.
	{http.MethodGet, "/v1/subscriptions", 70 << 10, 0, sharedHumanExact},
	{http.MethodPost, "/v1/subscriptions", 70 << 10, 70 << 10, sharedHumanExact},
	{http.MethodPost, "/v1/subscriptions/{subscription}", 70 << 10, 70 << 10, sharedHumanTrimmedID},
	{http.MethodPost, "/v1/subscriptions/{subscription}/delete", 70 << 10, 0, sharedHumanTrimmedID},
	{http.MethodGet, "/v1/board", 100000, 0, sharedHumanExact},
	{http.MethodPost, "/v1/board", 100000, 100000, sharedHumanExact},
	{http.MethodPost, "/v1/board/{id}", 100000, 100000, sharedHumanTrimmed},
	{http.MethodPost, "/v1/board/{id}/sync", 4096, 100000, noSharedHuman},
	{http.MethodPost, "/v1/board/{id}/start", 4096, 2048, noSharedHuman},
	{http.MethodGet, "/v1/forge-hosts", 4096, 0, noSharedHuman},
	{http.MethodPost, "/v1/forge-hosts", 4096, 16384, noSharedHuman},
	{http.MethodGet, "/v1/forge-connections", 4096, 0, noSharedHuman},
	{http.MethodPost, "/v1/forge-connections", 4096, 4096, noSharedHuman},
	{http.MethodGet, "/v1/forge-connections/{id}/issues", 4096, 0, noSharedHuman},
	{http.MethodPost, "/v1/forge-connections/{id}/issues", 4096, 70000, noSharedHuman},
	{http.MethodPost, "/v1/forge-connections/{id}/issues/{number}", 4096, 70000, noSharedHuman},
	{http.MethodPost, "/v1/forge-connections/{id}/issues/{number}/import", 4096, 70000, noSharedHuman},
	{http.MethodPost, "/v1/forge-connections/{id}/delete", 4096, 70000, noSharedHuman},
	{http.MethodGet, "/v1/work-items", 400000, 0, sharedHumanExact},
	{http.MethodPost, "/v1/work-items", 400000, 400000, noSharedHuman},
	{http.MethodGet, "/v1/work-items/{id}", 4096, 0, sharedHumanWorkItem},
	{http.MethodGet, "/v1/work-items/{id}/attempts", 4096, 0, sharedHumanAttempts},
	{http.MethodGet, "/v1/work-items/{id}/attempts/{attempt}/history", 4096, 0, noSharedHuman},
	{http.MethodGet, "/v1/work-items/{id}/attempts/{attempt}/changes", 4096, 0, noSharedHuman},
	{http.MethodGet, "/v1/work-items/{id}/files", 4096, 0, noSharedHuman},
	{http.MethodPost, "/v1/work-items/{id}/files", 4096, 2048, noSharedHuman},
	{http.MethodGet, "/v1/work-items/{id}/instruction", 4096, 0, noSharedHuman},
	{http.MethodGet, "/v1/work-items/{id}/artifacts/{artifact}/content", 4096, 0, noSharedHuman},
	{http.MethodPost, "/v1/work-items/{id}/responsibility", 4096, 4096, noSharedHuman},
	{http.MethodPost, "/v1/work-items/{id}/run", 4096, 2048, noSharedHuman},
	{http.MethodPost, "/v1/work-items/{id}/cancel", 4096, 1024, noSharedHuman},
	{http.MethodPost, "/v1/work-items/{id}/review", 8192, 1024, noSharedHuman},
	{http.MethodPost, "/v1/work-items/{id}/continue", 6 << 20, 6 << 20, noSharedHuman},
	{http.MethodPost, "/v1/work-items/{id}/image", 4096, 2048, noSharedHuman},
	{http.MethodPost, "/v1/work-items/{id}/data", 4096, 2048, noSharedHuman},
	{http.MethodGet, "/v1/connection-profiles", 32768, 0, noSharedHuman},
	{http.MethodPost, "/v1/connection-profiles", 32768, 32768, noSharedHuman},
	{http.MethodPost, "/v1/connection-profiles/{id}", 32768, 32768, noSharedHuman},
	{http.MethodPost, "/v1/connection-profiles/{id}/delete", 32768, 4096, noSharedHuman},
	{http.MethodGet, "/v1/projects", 4096, 0, sharedHumanAnyExact},
	{http.MethodPost, "/v1/projects", 4096, 1024, noSharedHuman},
	{http.MethodGet, "/v1/projects/{id}/audit-history", 4096, 0, noSharedHuman},
	{http.MethodGet, "/v1/projects/{id}/capabilities", 4096, 0, sharedHumanAnyTrimmed},
	{http.MethodGet, "/v1/projects/{id}/members", 4096, 0, sharedHumanAnyTrimmed},
	{http.MethodPost, "/v1/projects/{id}/members", 4096, 2048, noSharedHuman},
	{http.MethodPost, "/v1/projects/{id}/accept-invitation", 4096, 1024, sharedHumanAnyTrimmed},
	{http.MethodPost, "/v1/projects/{id}/delete", 4096, 1024, sharedHumanAnyTrimmed},
	{http.MethodPost, "/v1/projects/{id}/namespace", 4096, 0, noSharedHuman},
	{http.MethodPost, "/v1/projects/{id}/rename", 4096, 1024, sharedHumanAnyTrimmed},
	{http.MethodPost, "/v1/projects/{id}/coordination", 4096, 1024, noSharedHuman},
	{http.MethodPost, "/v1/projects/{id}/agent-access", 0, 1024, noSharedHuman},
	{http.MethodGet, "/v1/projects/{id}/hermes", 32768, 0, noSharedHuman},
	{http.MethodPost, "/v1/projects/{id}/hermes", 32768, 16384, noSharedHuman},
	{http.MethodPost, "/v1/projects/{id}/hermes/matrix-credential", 32768, 12288, noSharedHuman},
	{http.MethodPost, "/v1/projects/{id}/hermes/discover", 32768, 2048, noSharedHuman},
	{http.MethodPost, "/v1/projects/{id}/hermes/assistant", 32768, 1024, noSharedHuman},
	{http.MethodPost, "/v1/projects/{id}/hermes/matrix/start", 32768, 0, noSharedHuman},
	{http.MethodPost, "/v1/projects/{id}/hermes/matrix/stop", 32768, 0, noSharedHuman},
	{http.MethodGet, "/v1/project-invitations", 4096, 0, sharedHumanAnyExact},
	{http.MethodGet, "/v1/templates", 4096, 0, sharedHumanAnyExact},
	{http.MethodGet, "/v1/schedules", 4096, 0, noSharedHuman},
	{http.MethodPost, "/v1/schedules", 4096, 4096, noSharedHuman},
	{http.MethodPost, "/v1/schedules/{id}/enable", 4096, 1024, noSharedHuman},
	{http.MethodPost, "/v1/schedules/{id}/disable", 4096, 1024, noSharedHuman},
	{http.MethodPost, "/v1/schedules/{id}/run-now", 4096, 1024, noSharedHuman},
	{http.MethodPost, "/v1/schedules/{id}/update", 4096, 4096, noSharedHuman},
	{http.MethodPost, "/v1/schedules/{id}/delete", 4096, 1024, noSharedHuman},
	{http.MethodGet, "/v1/runs", 4096, 0, noSharedHuman},
	{http.MethodGet, "/v1/workspaces", 4096, 0, noSharedHuman},
	{http.MethodPost, "/v1/workspaces", 4096, 1024, noSharedHuman},
	{http.MethodGet, "/v1/workspaces/{id}/start-readiness", 4096, 0, noSharedHuman},
	{http.MethodGet, "/v1/workspaces/{id}/ports", 4096, 0, noSharedHuman},
	{http.MethodGet, "/v1/workspaces/{id}/models", 4096, 0, noSharedHuman},
	{http.MethodGet, "/v1/workspaces/{id}/setup", 4096, 0, noSharedHuman},
	{http.MethodPost, "/v1/workspaces/{id}/setup", 4096, 4096, noSharedHuman},
	{http.MethodGet, "/v1/workspaces/{id}/applications", 4096, 0, noSharedHuman},
	{http.MethodGet, "/v1/workspaces/{id}/artifacts", 4096, 0, noSharedHuman},
	{http.MethodGet, "/v1/workspaces/{id}/artifacts/{artifact}/content", 4096, 0, noSharedHuman},
	{http.MethodGet, "/v1/workspaces/{id}/services/{service}", 4096, 0, noSharedHuman},
	{http.MethodGet, "/v1/workspaces/{id}/logs", 4096, 0, noSharedHuman},
	{http.MethodGet, "/v1/workspaces/{id}/history", 4096, 0, noSharedHuman},
	{http.MethodGet, "/v1/workspaces/{id}/history/export", 4096, 0, noSharedHuman},
	{http.MethodGet, "/v1/workspaces/{id}/tasks/{task}", 70 << 10, 0, noSharedHuman},
	{http.MethodGet, "/v1/workspaces/{id}/tasks/{task}/result", 70 << 10, 0, noSharedHuman},
	{http.MethodPost, "/v1/workspaces/{id}/subscription/load", 4096, 4096, noSharedHuman},
	{http.MethodPost, "/v1/workspaces/{id}/subscription/unload", 4096, 4096, noSharedHuman},
	{http.MethodPost, "/v1/workspaces/{id}/terminals", 70 << 10, 32 << 10, noSharedHuman},
	{http.MethodPost, "/v1/workspaces/{id}/terminals/{terminal}/exchange", 70 << 10, 0, noSharedHuman},
	{http.MethodPost, "/v1/workspaces/{id}/terminals/{terminal}/close", 70 << 10, 0, noSharedHuman},
	{http.MethodPost, "/v1/workspaces/{id}/applications", 4096, 8192, noSharedHuman},
	{http.MethodPost, "/v1/workspaces/{id}/retained-previews", 4096, 2 << 20, noSharedHuman},
	{http.MethodPost, "/v1/workspaces/{id}/services/{service}/revoke", 4096, 0, noSharedHuman},
	{http.MethodPost, "/v1/workspaces/{id}/services/{service}/release", 4096, 0, noSharedHuman},
	{http.MethodPost, "/v1/workspaces/{id}/history/feedback", 4096, 2048, noSharedHuman},
	{http.MethodPost, "/v1/workspaces/{id}/history/purpose", 4096, 2048, noSharedHuman},
	{http.MethodPost, "/v1/workspaces/{id}/delete", 4096, 1024, noSharedHuman},
	{http.MethodPost, "/v1/workspaces/{id}/start", 4096, 1024, noSharedHuman},
	{http.MethodPost, "/v1/workspaces/{id}/stop", 4096, 1024, noSharedHuman},
	{http.MethodPost, "/v1/workspaces/{id}/force-stop", 0, 1024, noSharedHuman},
	{http.MethodPost, "/v1/workspaces/{id}/desktop-pair", 4096, 0, noSharedHuman},
	{http.MethodPost, "/v1/workspaces/{id}/enable-t3", 4096, 1024, noSharedHuman},
	{http.MethodPost, "/v1/workspaces/{id}/settings", 32768, 32768, noSharedHuman},
	{http.MethodPost, "/v1/workspaces/{id}/tasks", 70 << 10, 70 << 10, noSharedHuman},
	{http.MethodPost, "/v1/workspaces/{id}/tasks/{task}/cancel", 70 << 10, 1024, noSharedHuman},
	{http.MethodPost, "/v1/workspaces/{id}/terminals/{terminal}/{action}", 0, 32 << 10, noSharedHuman},
	{http.MethodPost, "/v1/workspaces/{id}/services/{service}/fetch", 0, 2 << 20, noSharedHuman},
	{http.MethodGet, "/v1/workspace-logs", 4096, 0, noSharedHuman},
	{http.MethodPost, "/v1/project-exchanges", 4096, 4096, noSharedHuman},
	{http.MethodPost, "/v1/project-exchanges/{id}/approve", 4096, 1024, noSharedHuman},
	{http.MethodPost, "/v1/project-exchanges/{id}/revoke", 4096, 1024, noSharedHuman},
	{http.MethodPost, "/v1/project-exchanges/{id}/exports", 100000, 70000, noSharedHuman},
	{http.MethodGet, "/v1/exchange-inbox", 4096, 0, noSharedHuman},
	{http.MethodGet, "/v1/exchange-exports/{id}", 4096, 0, noSharedHuman},
	{http.MethodPost, "/v1/exchange-exports/{id}/submit", 4096, 1024, noSharedHuman},
	{http.MethodPost, "/v1/note-publications", 100000, 70000, noSharedHuman},
	{http.MethodPost, "/v1/file-handoffs", 4096, 4096, noSharedHuman},
	{http.MethodPost, "/internal/subscriptions/resolve", 0, 4096, noSharedHuman},
}

var apiBodyLimits = func() map[string]int64 {
	limits := make(map[string]int64)
	for _, route := range managementRoutes {
		if route.apiLimit != 0 {
			limits[route.method+" "+route.pattern] = route.apiLimit
		}
	}
	return limits
}()

// APIBodyLimit uses the pattern selected by ServeMux. Looking at the decoded
// URL instead would apply different limits to some escaped paths.
func APIBodyLimit(pattern string) int64 {
	return apiBodyLimits[pattern]
}

// DashboardBodyLimit returns zero for requests outside the management surface.
func DashboardBodyLimit(method, path string) int64 {
	if !strings.HasPrefix(path, "/") || strings.Contains(path, "//") || strings.HasSuffix(path, "/") {
		return 0
	}
	path = path[1:]
	for _, route := range managementRoutes {
		if route.dashboardLimit != 0 && route.method == method &&
			matchRouteSegments(path, route.pattern[len("/v1/"):], true) {
			return route.dashboardLimit
		}
	}
	return 0
}

// SharedHumanRouteAllowed preserves the authorization gate's matching rules.
// ServeMux still resolves methods and paths, then handlers enforce membership.
func SharedHumanRouteAllowed(method, path string) bool {
	_, relativePath, _ := strings.Cut(strings.Trim(path, "/"), "/")
	for _, route := range managementRoutes {
		if route.sharedHuman == noSharedHuman {
			continue
		}
		if route.method != method && route.sharedHuman != sharedHumanAnyExact && route.sharedHuman != sharedHumanAnyTrimmed {
			continue
		}
		switch route.sharedHuman {
		case sharedHumanExact, sharedHumanAnyExact:
			if path == route.pattern {
				return true
			}
		case sharedHumanTrimmed, sharedHumanAnyTrimmed, sharedHumanTrimmedID:
			// The outer API mux provides the namespace. The original gate
			// compared the remaining segments after trimming outer slashes.
			if matchRouteSegments(relativePath, route.pattern[len("/v1/"):], route.sharedHuman == sharedHumanTrimmedID) {
				return true
			}
		case sharedHumanWorkItem:
			prefix, _, _ := strings.Cut(route.pattern, "{")
			if strings.HasPrefix(path, prefix) && matchRouteSegments(relativePath, route.pattern[len("/v1/"):], false) {
				return true
			}
		case sharedHumanAttempts:
			// The gate historically matched the final suffix at any depth.
			prefix, tail, _ := strings.Cut(route.pattern, "{")
			_, suffix, _ := strings.Cut(tail, "}")
			if strings.HasPrefix(path, prefix) && strings.HasSuffix(path, suffix) {
				return true
			}
		}
	}
	return false
}

func matchRouteSegments(path, pattern string, validIDs bool) bool {
	for {
		part, rest, more := strings.Cut(path, "/")
		segment, remainingPattern, morePattern := strings.Cut(pattern, "/")
		if more != morePattern {
			return false
		}
		if strings.HasPrefix(segment, "{") {
			if validIDs && !ValidID(part) {
				return false
			}
		} else if segment != part {
			return false
		}
		if !more {
			return true
		}
		path, pattern = rest, remainingPattern
	}
}
