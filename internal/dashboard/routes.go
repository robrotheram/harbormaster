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
		case "/subscriptions", "/subscription", "/board", "/forge-hosts", "/forge-connections", "/work-items", "/connection-profiles", "/projects", "/schedules", "/workspaces":
			return true
		case "/exchange-inbox", "/workspace-logs", "/templates", "/agent-homes", "/runs":
			return get
		case "/project-exchanges", "/note-publications", "/file-handoffs":
			return post
		}
		return false
	}
	if path == "/subscription/delete" {
		return post
	}
	if len(parts) == 4 && parts[0] == "workspaces" && shared.ValidID(parts[1]) && parts[2] == "subscription" {
		return post && (parts[3] == "load" || parts[3] == "unload")
	}
	if !shared.ValidID(parts[1]) {
		return false
	}
	switch parts[0] {
	case "subscriptions":
		return post && (len(parts) == 2 || len(parts) == 3 && parts[2] == "delete")
	case "board":
		return post && (len(parts) == 2 || len(parts) == 3 && parts[2] == "sync")
	case "forge-connections":
		if len(parts) == 3 {
			return parts[2] == "issues" || post && parts[2] == "delete"
		}
		if len(parts) == 4 {
			return post && parts[2] == "issues" && shared.ValidID(parts[3])
		}
		return len(parts) == 5 && post && parts[2] == "issues" && shared.ValidID(parts[3]) && parts[4] == "import"

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
		case "attempts", "instruction":
			return get
		case "run", "cancel", "review", "continue", "image", "data":
			return post
		}
		return false
	case "connection-profiles":
		return post && (len(parts) == 2 || len(parts) == 3 && parts[2] == "delete")
	case "project-exchanges":
		return len(parts) == 3 && post && (parts[2] == "approve" || parts[2] == "revoke" || parts[2] == "exports")
	case "exchange-exports":
		return len(parts) == 2 && get || len(parts) == 3 && post && parts[2] == "submit"
	case "projects":
		if len(parts) == 3 && (parts[2] == "delete" || parts[2] == "namespace") {
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
		return len(parts) == 3 && post && (parts[2] == "enable" || parts[2] == "disable" || parts[2] == "run-now" || parts[2] == "update" || parts[2] == "delete")
	case "workspaces":
		if len(parts) < 3 {
			return false
		}
		switch parts[2] {
		case "terminals":
			return post && (len(parts) == 3 || len(parts) == 5 && shared.ValidID(parts[3]) && (parts[4] == "exchange" || parts[4] == "close"))
		case "start-readiness":
			return len(parts) == 3 && get
		case "ports":
			return len(parts) == 3 && get
		case "applications":
			return len(parts) == 3 && (get || post)
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
