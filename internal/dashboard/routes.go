package dashboard

import (
	"net/http"
	"strings"

	"agent-workbench/internal/shared"
)

// allowedUIRoute is the local operator's exact management API surface. Never
// turn the ingress into a generic control-plane proxy when adding a feature.
func allowedUIRoute(method, path string) bool {
	if !strings.HasPrefix(path, "/") || strings.Contains(path, "//") || strings.HasSuffix(path, "/") {
		return false
	}
	get, post := method == http.MethodGet, method == http.MethodPost
	if !get && !post {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) == 1 {
		switch path {
		case "/connection-profiles", "/tool-profiles", "/projects", "/schedules", "/workspaces":
			return true
		case "/workspace-logs", "/profiles", "/templates", "/agent-homes", "/runs":
			return get
		case "/file-handoffs":
			return post
		}
		return false
	}
	if !shared.ValidID(parts[1]) {
		return false
	}
	switch parts[0] {
	case "connection-profiles", "tool-profiles":
		return len(parts) == 2 && post
	case "projects":
		if len(parts) < 3 || parts[2] != "hermes" {
			return false
		}
		if len(parts) == 3 {
			return true
		}
		if !post {
			return false
		}
		if len(parts) == 4 {
			return parts[3] == "matrix-credential" || parts[3] == "discover" || parts[3] == "assistant"
		}
		return len(parts) == 5 && parts[3] == "matrix" && (parts[4] == "start" || parts[4] == "stop")
	case "schedules":
		return len(parts) == 3 && post && (parts[2] == "enable" || parts[2] == "disable" || parts[2] == "run-now")
	case "workspaces":
		if len(parts) < 3 {
			return false
		}
		switch parts[2] {
		case "logs":
			return len(parts) == 3 && get
		case "history":
			return len(parts) == 3 && get || len(parts) == 4 && (get && parts[3] == "export" || post && (parts[3] == "feedback" || parts[3] == "purpose"))
		case "start", "stop", "desktop-pair", "enable-t3", "settings":
			return len(parts) == 3 && post
		case "tasks":
			if len(parts) == 3 {
				return post
			}
			if !shared.ValidID(parts[3]) {
				return false
			}
			return len(parts) == 4 && get || len(parts) == 5 && (parts[4] == "result" && get || parts[4] == "cancel" && post)
		}
	}
	return false
}
