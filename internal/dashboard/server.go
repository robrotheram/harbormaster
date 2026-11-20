// Package dashboard serves the React workbench and its bounded API adapters.
package dashboard

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"
)

const snapshotPath = "/dashboard-api/v1/snapshot"
const maxSnapshotBytes = 4 << 20

// NewHandler fixes the upstream origin at startup. Incoming headers, queries and
// paths never influence the upstream request; redirects are never followed.
func NewHandler(controlplaneReadURL, readToken string) (http.Handler, error) {
	u, err := url.Parse(controlplaneReadURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("CONTROLPLANE_READ_URL must be an HTTP origin without credentials, query or path")
	}
	u.Path = snapshotPath
	client := &http.Client{Timeout: 4 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
			return
		}
		if r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.EscapedPath() != r.URL.Path {
			writeError(w, http.StatusNotFound, "NOT_FOUND")
			return
		}
		switch r.URL.Path {
		case "/healthz":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"status":"ok"}`)
		case snapshotPath:
			req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, u.String(), nil)
			if err != nil {
				writeError(w, 503, "SNAPSHOT_UNAVAILABLE")
				return
			}
			req.Header.Set("Accept", "application/json")
			if readToken != "" {
				req.Header.Set("Authorization", "Bearer "+readToken)
			}
			res, err := client.Do(req)
			if err != nil {
				writeError(w, 503, "SNAPSHOT_UNAVAILABLE")
				return
			}
			defer res.Body.Close()
			if res.StatusCode != http.StatusOK {
				writeError(w, 503, "SNAPSHOT_UNAVAILABLE")
				return
			}
			body, err := io.ReadAll(io.LimitReader(res.Body, maxSnapshotBytes+1))
			// Buffer and validate before writing: never forward partial or non-JSON errors.
			var object map[string]json.RawMessage
			if err != nil || len(body) > maxSnapshotBytes || json.Unmarshal(body, &object) != nil || object == nil {
				writeError(w, 503, "SNAPSHOT_UNAVAILABLE")
				return
			}
			if r.Context().Value(humanIdentityKey{}) != nil {
				body, err = serviceHealthSummary(body)
				if err != nil {
					writeError(w, 503, "SNAPSHOT_UNAVAILABLE")
					return
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		case "/", "/workspaces":
			http.Redirect(w, r, "/app/", http.StatusTemporaryRedirect)
		case "/getting-started":
			http.Redirect(w, r, "/app/projects/default/help", http.StatusTemporaryRedirect)
		default:
			writeError(w, http.StatusNotFound, "NOT_FOUND")
		}
	}), nil
}

func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		Retryable bool   `json:"retryable"`
	}{code, "Dashboard request unavailable", status == 503})
}

// Human members can observe shared service health, never global worker inventory,
// task details, quotas, addresses or arbitrary diagnostic payloads.
func serviceHealthSummary(body []byte) ([]byte, error) {
	type observation struct {
		Name   string    `json:"name"`
		Status string    `json:"status"`
		Reason string    `json:"reason_code,omitempty"`
		At     time.Time `json:"observed_at"`
	}
	var snapshot struct {
		Generated time.Time `json:"generated_at"`
		Services  struct {
			Items []observation `json:"items"`
		} `json:"services"`
	}
	if err := json.Unmarshal(body, &snapshot); err != nil {
		return nil, err
	}
	services := []observation{}
	for _, service := range snapshot.Services.Items {
		switch service.Name {
		case "Control plane", "Platform metadata", "Runtime controller", "Inference gateway", "Credential broker", "MCP":
		default:
			continue
		}
		switch service.Status {
		case "healthy", "degraded", "unavailable", "unhealthy", "failed", "down", "unknown":
		default:
			service.Status = "unknown"
		}
		switch service.Reason {
		case "PROCESS_READY", "HEALTH_CHECK_FAILED", "NOT_PROBED":
		default:
			service.Reason = ""
		}
		services = append(services, service)
	}
	return json.Marshal(map[string]any{"scope": "services", "generated_at": snapshot.Generated, "services": map[string]any{"items": services}})
}
