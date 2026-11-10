package firecracker

// This broker is bound to one VM generation, not to a guest-selected host.
import (
	"agent-workbench/internal/shared"
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const projectMCPAddress = "http://127.0.0.1:18085/project-mcp"
const projectMCPSigningFile = "/etc/agent-workbench/firecracker/project_mcp_signing_key"

func (f *Firecracker) projectMCP(v *fcVM, secret string, client *http.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		f.mu.Lock()
		current := f.vms[v.ID] == v && v.running && !v.stopping && v.Generation != ""
		f.mu.Unlock()
		if !current || v.Spec.AgentKind != "hermes" || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+f.grant(v))) != 1 {
			http.Error(w, "unauthorized", 401)
			return
		}
		if secret == "" {
			http.Error(w, "MCP not provisioned", 503)
			return
		}
		if r.Method != "POST" || r.URL.Path != "/mcp" || r.URL.RawQuery != "" {
			http.Error(w, "forbidden", 403)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 512<<10))
		if err != nil {
			http.Error(w, "invalid body", 400)
			return
		}
		var rpc struct {
			Method string `json:"method"`
			Params struct {
				Name string `json:"name"`
			} `json:"params"`
		}
		if json.Unmarshal(body, &rpc) != nil {
			http.Error(w, "invalid RPC", 400)
			return
		}
		switch rpc.Method {
		case "initialize", "notifications/initialized", "ping", "tools/list":
		case "tools/call":
			if !shared.ProjectToolAllowed(rpc.Params.Name) {
				http.Error(w, "forbidden tool", 403)
				return
			}
		default:
			http.Error(w, "forbidden method", 403)
			return
		}
		// Renew internally on every request. A guest never receives the signing key,
		// management token, or a transferable project credential.
		token, err := shared.IssueProjectAccess(secret, v.Spec.ProjectID, time.Now().Add(time.Minute))
		if err != nil {
			http.Error(w, "unavailable", 503)
			return
		}
		req, err := http.NewRequestWithContext(r.Context(), "POST", projectMCPAddress, bytes.NewReader(body))
		if err != nil {
			http.Error(w, "unavailable", 503)
			return
		}
		req.Header.Set("Authorization", "Bearer "+token)
		for _, key := range []string{"Content-Type", "Accept", "Mcp-Protocol-Version"} {
			if value := r.Header.Get(key); value != "" {
				req.Header.Set(key, value)
			}
		}
		res, err := client.Do(req)
		if err != nil {
			http.Error(w, "unavailable", 503)
			return
		}
		defer res.Body.Close()
		w.Header().Set("Content-Type", res.Header.Get("Content-Type"))
		w.WriteHeader(res.StatusCode)
		io.Copy(w, io.LimitReader(res.Body, 2<<20))
	})
}

func projectMCPSecret() string {
	if trustedFile(projectMCPSigningFile) != nil {
		return ""
	}
	body, err := os.ReadFile(projectMCPSigningFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(body))
}
