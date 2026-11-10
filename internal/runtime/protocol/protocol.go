// Package protocol defines the controller/workspace contract without depending
// on any compute provider. Providers own enforcement; these helpers own wire rules.
package protocol

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"agent-workbench/internal/shared"
)

// APIError is an unsuccessful response from a provider API, not a workspace error.
type APIError struct{ Status int }

func (e APIError) Error() string { return fmt.Sprintf("runtime API status %d", e.Status) }

// Grant binds authority to a workspace generation. Keep this derivation stable
// across provider upgrades; changing it revokes existing generation grants.
func Grant(secret, workspace, generation string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(workspace + ":" + generation))
	return hex.EncodeToString(h.Sum(nil))
}

func AllowedAgentPath(method, path string) bool {
	partsFactory := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(partsFactory) >= 2 && partsFactory[0] == "v1" {
		switch partsFactory[1] {
		case "coordination":
			if len(partsFactory) == 3 && partsFactory[2] == "notes" {
				return method == "POST"
			}
			return len(partsFactory) == 4 && partsFactory[2] == "blobs" && shared.ValidID(partsFactory[3]) && (method == "GET" || method == "POST")
		case "services":
			if len(partsFactory) == 2 {
				return method == "POST"
			}
			if len(partsFactory) == 3 && (partsFactory[2] == "reconcile" || partsFactory[2] == "receipt") {
				return method == "POST"
			}
			if !shared.ValidID(partsFactory[2]) {
				return false
			}
			return len(partsFactory) == 3 && method == "GET" || len(partsFactory) == 4 && method == "POST" && (partsFactory[3] == "fetch" || partsFactory[3] == "revoke")
		case "artifacts":
			if len(partsFactory) == 2 {
				return method == "GET" || method == "POST"
			}
			return len(partsFactory) == 4 && shared.ValidID(partsFactory[2]) && partsFactory[3] == "content" && method == "GET"
		case "recipe-jobs":
			if len(partsFactory) == 2 {
				return method == "POST" || method == "GET"
			}
			if !shared.ValidID(partsFactory[2]) {
				return false
			}
			return len(partsFactory) == 3 && method == "GET" || len(partsFactory) == 4 && partsFactory[3] == "cancel" && method == "POST"
		}
	}
	if (method == "POST" && (path == "/v1/matrix/start" || path == "/v1/matrix/stop")) || (method == "GET" && path == "/v1/matrix/status") {
		return true
	}
	if method == "GET" && path == "/v1/runtime-capabilities" {
		return true
	}
	if path == "/v1/console/enable" || path == "/v1/files/read" || path == "/v1/files/write-new" || path == "/v1/tasks/from-file" {
		return method == "POST"
	}
	if path == "/v1/activity" {
		return method == "GET"
	}
	if path == "/v1/tasks" {
		return method == "GET" || method == "POST"
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 3 || parts[0] != "v1" || parts[1] != "tasks" || !shared.ValidID(parts[2]) {
		return false
	}
	if len(parts) == 3 {
		return method == "GET"
	}
	return len(parts) == 4 && ((parts[3] == "result" && method == "GET") || (parts[3] == "cancel" && method == "POST"))
}

// AgentQuery validates parameters by endpoint before any request reaches a guest.
// It is shared by every controller so history and task protocols evolve together.
func AgentQuery(method, path, raw string) (string, error) {
	q, err := url.ParseQuery(raw)
	if err != nil || len(raw) > 8192 {
		return "", fmt.Errorf("invalid workspace query")
	}
	for key, values := range q {
		if method != "GET" || len(values) != 1 {
			return "", fmt.Errorf("invalid workspace query")
		}
		value := values[0]
		allowed := false
		if path == "/v1/tasks" {
			switch key {
			case "history":
				allowed = value == "1"
			case "order":
				allowed = value == "asc" || value == "desc"
			case "episode":
				allowed = shared.ValidID(value)
			case "after", "before", "until", "limit":
				n, e := strconv.ParseInt(value, 10, 64)
				allowed = e == nil && n >= 0
			}
		} else if strings.HasSuffix(path, "/result") && AllowedAgentPath(method, path) && (key == "offset" || key == "limit") {
			n, e := strconv.ParseInt(value, 10, 64)
			allowed = e == nil && n >= 0
		}
		if !allowed {
			return "", fmt.Errorf("invalid workspace query")
		}
	}
	return q.Encode(), nil
}

func AgentBodyLimit(path string) int64 {
	if strings.HasPrefix(path, "/v1/coordination/") {
		return 100000
	}
	if path == "/v1/services" {
		return 24 << 20
	}
	if path == "/v1/files/write-new" {
		return 1500 << 10
	}
	return 70 << 10
}

// AgentResponseLimit keeps large immutable artifact transfers bounded without
// enlarging ordinary metadata/tool responses. Controller routes include /agent.
func AgentResponseLimit(path string) int64 {
	if _, tail, ok := strings.Cut(path, "/agent/"); ok {
		path = "/" + tail
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) == 4 && parts[0] == "v1" && parts[1] == "services" && shared.ValidID(parts[2]) && parts[3] == "fetch" {
		return 3 << 20
	}
	if len(parts) == 4 && parts[0] == "v1" && parts[1] == "artifacts" && shared.ValidID(parts[2]) && parts[3] == "content" {
		return 32 << 20
	}
	return 2 << 20
}
