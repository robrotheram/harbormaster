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
		case "/work-items", "/connection-profiles", "/projects", "/schedules", "/workspaces":
			return true
		case "/exchange-inbox", "/workspace-logs", "/templates", "/agent-homes", "/runs":
			return get
		case "/project-exchanges", "/note-publications", "/file-handoffs":
			return post
		}
		return false
	}
	if !shared.ValidID(parts[1]) {
		return false
	}
	switch parts[0] {
	case "work-items":
		if len(parts) == 5 && parts[2] == "artifacts" && shared.ValidID(parts[3]) && parts[4] == "content" {
			return get
		}
		if len(parts) == 2 {
			return get
		}
		if len(parts) != 3 {
			return false
		}
		switch parts[2] {
		case "attempts":
			return get
		case "run", "cancel", "review":
			return post
		}
		return false
	case "connection-profiles":
		return len(parts) == 2 && post
	case "project-exchanges":
		return len(parts) == 3 && post && (parts[2] == "approve" || parts[2] == "revoke" || parts[2] == "exports")
	case "exchange-exports":
		return len(parts) == 2 && get || len(parts) == 3 && post && parts[2] == "submit"
	case "projects":
		if len(parts) == 3 && parts[2] == "delete" {
			return post
		}
		if len(parts) == 3 && parts[2] == "coordination" {
			return post
		}
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
		case "recipe-jobs":
			return len(parts) == 3 && (get || post) || len(parts) == 4 && shared.ValidID(parts[3]) && get || len(parts) == 5 && shared.ValidID(parts[3]) && parts[4] == "cancel" && post
		case "artifacts":
			return len(parts) == 3 && get || len(parts) == 5 && shared.ValidID(parts[3]) && parts[4] == "content" && get
		case "retained-previews":
			return len(parts) == 3 && post
		case "services":
			return len(parts) == 4 && shared.ValidID(parts[3]) && get || len(parts) == 5 && shared.ValidID(parts[3]) && (parts[4] == "revoke" || parts[4] == "release") && post
		case "logs":
			return len(parts) == 3 && get
		case "history":
			return len(parts) == 3 && get || len(parts) == 4 && (get && parts[3] == "export" || post && (parts[3] == "feedback" || parts[3] == "purpose"))
		case "delete", "start", "stop", "desktop-pair", "enable-t3", "settings":
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
