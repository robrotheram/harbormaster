package gateway

import (
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func verifier(_ context.Context, token, audience string) (Identity, error) {
	if token != "workspace-token" || audience != "inference" {
		return Identity{}, errors.New("denied")
	}
	return Identity{WorkspaceID: "w1", ProjectID: "default", PrincipalID: "alice"}, nil
}
func call(h http.Handler, body, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("X-Principal-ID", "mallory")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

const validRequest = `{"model":"allowed","messages":[{"role":"user","content":"hello"}]}`

func TestFixtureAndDenials(t *testing.T) {
	h, err := NewHandler(Config{Mode: "fixture", Models: []string{"allowed"}, Verify: verifier})
	if err != nil {
		t.Fatal(err)
	}
	a := call(h, validRequest, "workspace-token")
	b := call(h, validRequest, "workspace-token")
	if a.Code != 200 || a.Body.String() != b.Body.String() {
		t.Fatalf("fixture failed: %d", a.Code)
	}
	var response struct {
		Model   string
		Choices []struct{ Message struct{ Content string } }
	}
	if json.Unmarshal(a.Body.Bytes(), &response) != nil || response.Model != "fixture" || !strings.Contains(response.Choices[0].Message.Content, "SYNTHETIC FIXTURE") {
		t.Fatal("fixture not labeled")
	}
	changed := call(h, strings.Replace(validRequest, "hello", "different", 1), "workspace-token")
	if changed.Body.String() == a.Body.String() {
		t.Fatal("fixture ignored messages")
	}
	for _, tc := range []struct {
		name, body, token string
		status            int
	}{
		{"denied grant", validRequest, "invalid", 401},
		{"denied model", strings.Replace(validRequest, "allowed", "other", 1), "workspace-token", 404},
		{"streaming", `{"model":"allowed","messages":[],"stream":true}`, "workspace-token", 400},
		{"arbitrary url", `{"model":"allowed","messages":[],"url":"https://attacker.test"}`, "workspace-token", 400},
		{"spoofed identity", `{"model":"allowed","messages":[],"principal_id":"alice"}`, "workspace-token", 400},
		{"oversized", strings.Repeat("x", (1<<20)+1), "workspace-token", 400},
		{"trailing json", validRequest + `{}`, "workspace-token", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if w := call(h, tc.body, tc.token); w.Code != tc.status {
				t.Fatalf("got %d want %d", w.Code, tc.status)
			}
		})
	}
	for _, method := range []string{"CONNECT", "GET", "PUT"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, "/v1/chat/completions", nil))
		if w.Code < 400 {
			t.Fatalf("method %s accepted", method)
		}
	}
}
func tlsPolicy(t *testing.T, s *httptest.Server) NetworkPolicy {
	t.Helper()
	file := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(file, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	return NetworkPolicy{AllowedCIDRs: []string{"127.0.0.1/32"}, AllowPrivate: true, CAFile: file}
}
func TestUpstreamAndNetworkPolicy(t *testing.T) {
	var hits atomic.Int32
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer upstream-secret" || r.Header.Get("X-Principal-ID") != "" {
			t.Error("unexpected upstream routing or headers")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"allowed","choices":[]}`))
	}))
	defer upstream.Close()
	policy := tlsPolicy(t, upstream)
	config := Config{Mode: "upstream", Models: []string{"allowed"}, Origin: upstream.URL, APIKey: "upstream-secret", Network: policy, Verify: verifier}
	h, err := NewHandler(config)
	if err != nil {
		t.Fatal(err)
	}
	if w := call(h, validRequest, "workspace-token"); w.Code != 200 || w.Body.String() != `{"model":"allowed","choices":[]}` {
		t.Fatalf("upstream failed %d %s", w.Code, w.Body.String())
	}
	for _, tc := range []struct {
		name   string
		policy NetworkPolicy
	}{
		{"private not explicit", NetworkPolicy{AllowedCIDRs: []string{"0.0.0.0/0"}, CAFile: policy.CAFile}},
		{"cidr mismatch", NetworkPolicy{AllowedCIDRs: []string{"192.0.2.0/24"}, AllowPrivate: true, CAFile: policy.CAFile}},
		{"untrusted tls", NetworkPolicy{AllowedCIDRs: policy.AllowedCIDRs, AllowPrivate: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := config
			c.Network = tc.policy
			h, err := NewHandler(c)
			if err != nil {
				t.Fatal(err)
			}
			w := call(h, validRequest, "workspace-token")
			if w.Code != 503 || strings.Contains(w.Body.String(), "upstream-secret") {
				t.Fatal("unsafe upstream failure")
			}
		})
	}
	if hits.Load() != 1 {
		t.Fatal("blocked destination reached")
	}
	for _, origin := range []string{"http://example.test", "https://example.test/alternate", "https://user:password@example.test", "https://example.test?url=x"} {
		c := config
		c.Origin = origin
		if _, err := NewHandler(c); err == nil {
			t.Fatalf("accepted origin %q", origin)
		}
	}
}
func TestRedirectNotFollowed(t *testing.T) {
	var targetHits atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetHits.Add(1) }))
	defer target.Close()
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer source.Close()
	h, err := NewHandler(Config{Mode: "upstream", Models: []string{"allowed"}, Origin: source.URL, APIKey: "secret", Network: tlsPolicy(t, source), Verify: verifier})
	if err != nil {
		t.Fatal(err)
	}
	if w := call(h, validRequest, "workspace-token"); w.Code != 503 {
		t.Fatalf("got %d", w.Code)
	}
	if targetHits.Load() != 0 {
		t.Fatal("redirect followed")
	}
}
func TestControllerVerifier(t *testing.T) {
	var gotAudience string
	controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/grants/verify" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer service-token" {
			t.Error("wrong verify request")
		}
		var body struct{ Token, Audience string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotAudience = body.Audience
		if body.Token != "workspace-token" {
			w.WriteHeader(403)
			return
		}
		shared.JSON(w, 200, Identity{WorkspaceID: "w1", ProjectID: "default", PrincipalID: "alice"})
	}))
	defer controller.Close()
	verify, err := NewVerifier(controller.URL, "service-token")
	if err != nil {
		t.Fatal(err)
	}
	if id, err := verify(context.Background(), "workspace-token", "broker"); err != nil || id.PrincipalID != "alice" || gotAudience != "broker" {
		t.Fatal("verification failed")
	}
	if _, err := verify(context.Background(), "bad", "broker"); err == nil {
		t.Fatal("denied grant accepted")
	}
	controller.Close()
	if _, err := verify(context.Background(), "workspace-token", "inference"); err == nil {
		t.Fatal("controller outage accepted")
	}
}
func TestIncompleteIdentityDenied(t *testing.T) {
	h, err := NewHandler(Config{Mode: "fixture", Models: []string{"allowed"}, Verify: func(context.Context, string, string) (Identity, error) { return Identity{PrincipalID: "alice"}, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if w := call(h, validRequest, "workspace-token"); w.Code != 401 {
		t.Fatal("incomplete identity accepted")
	}
}

func TestConfigFromEnvExample(t *testing.T) {
	tokenPath := filepath.Join(t.TempDir(), "service-token")
	if err := os.WriteFile(tokenPath, []byte("test-service-token\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GATEWAY_CONFIG", "config.example.json")
	t.Setenv("GATEWAY_MODE", "fixture")
	t.Setenv("GATEWAY_SERVICE_TOKEN_FILE", tokenPath)
	t.Setenv("CONTROLLER_URL", "")
	c, err := ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewHandler(c); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GATEWAY_MODE", "upstream")
	t.Setenv("INFERENCE_API_KEY_FILE", filepath.Join(t.TempDir(), "missing"))
	if _, err := ConfigFromEnv(); err == nil {
		t.Fatal("missing upstream secret accepted")
	}
}
