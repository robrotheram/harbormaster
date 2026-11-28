package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type subscriptionRoundTrip func(*http.Request) (*http.Response, error)

func (f subscriptionRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSubscriptionCredentialBoundary(t *testing.T) {
	var expiry atomic.Int64
	expiry.Store(time.Now().Add(time.Hour).Unix())
	var reads, calls atomic.Int32
	var slow atomic.Bool
	cfg := Config{Mode: "fixture", Models: []string{"fixture"},
		Subscriptions: &SubscriptionConfig{Providers: map[string]SubscriptionProvider{"codex": {Models: []string{"approved-codex"}, Network: NetworkPolicy{AllowedCIDRs: []string{"203.0.113.7/32"}}}, "claude": {Models: []string{"approved-claude"}, Network: NetworkPolicy{AllowedCIDRs: []string{"203.0.113.8/32"}}}}},
		Verify: func(_ context.Context, token, audience string) (Identity, error) {
			if token != "workspace-grant" || audience != "inference" {
				return Identity{}, ErrDenied
			}
			return Identity{PrincipalID: "alice", ProjectID: "default", WorkspaceID: "ws-a"}, nil
		},
		ResolveSubscription: func(_ context.Context, workspace, provider, grant string) (subscriptionCredential, error) {
			if workspace != "ws-a" || (provider != "codex" && provider != "claude") || grant != "workspace-grant" {
				t.Error("unexpected credential lookup")
				return subscriptionCredential{}, ErrDenied
			}
			reads.Add(1)
			return subscriptionCredential{AccessToken: "upstream-secret-marker", AccountID: "real-account", ExpiresAt: expiry.Load()}, nil
		},
	}
	cfg.subscriptionHTTP = &http.Client{CheckRedirect: NoRedirect, Transport: subscriptionRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if slow.Load() {
			time.Sleep(250 * time.Millisecond)
		}
		if r.Header.Get("Authorization") != "Bearer upstream-secret-marker" || r.Header.Get("X-Untrusted-Token") != "" || r.Header.Get("X-Api-Key") != "" || r.Header.Get("X-Identity") != "" {
			t.Error("credential boundary failed")
		}
		switch r.URL.Host {
		case "chatgpt.com":
			if r.URL.Path != "/backend-api/codex/responses" || r.Header.Get("ChatGPT-Account-Id") != "real-account" {
				t.Error("wrong Codex authority")
			}
		case "api.anthropic.com":
			if r.URL.Path != "/v1/messages" || r.Header.Get("Anthropic-Beta") != "oauth-capability,example-beta" {
				t.Error("lost native Claude protocol")
			}
		default:
			t.Error("arbitrary upstream")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: native-result\n\n"))}, nil
	})}
	h, err := NewHandler(cfg)
	if err != nil {
		t.Fatal(err)
	}
	request := func(path, token, model string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("POST", path, strings.NewReader(`{"model":"`+model+`","stream":true}`))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("ChatGPT-Account-Id", "forged")
		r.Header.Set("X-Untrusted-Token", "forged")
		r.Header.Set("X-Api-Key", "forged")
		r.Header.Set("X-Identity", "bob")
		r.Header.Set("Anthropic-Beta", "oauth-capability,example-beta")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if strings.Contains(w.Body.String(), "secret-marker") {
			t.Fatal("secret reflected")
		}
		return w
	}
	for _, tt := range []struct{ path, model string }{{"/codex/responses", "approved-codex"}, {"/claude/v1/messages?beta=true", "approved-claude"}} {
		if w := request(tt.path, "workspace-grant", tt.model); w.Code != 200 || !strings.Contains(w.Body.String(), "native-result") {
			t.Fatalf("native request failed: %d", w.Code)
		}
	}
	before := reads.Load()
	for _, tt := range []struct{ path, token, model string }{{"/codex/responses", "invalid", "approved-codex"}, {"/codex/responses", "workspace-grant", "other-model"}, {"/codex/responses?url=https://evil.invalid", "workspace-grant", "approved-codex"}, {"/codex/arbitrary", "workspace-grant", "approved-codex"}} {
		if w := request(tt.path, tt.token, tt.model); w.Code == 200 {
			t.Fatal("denial expected")
		}
	}
	if reads.Load() != before {
		t.Fatal("invalid request reached the credential resolver")
	}
	t.Run("native responses survive the short server write timeout", func(t *testing.T) {
		slow.Store(true)
		defer slow.Store(false)
		server := httptest.NewUnstartedServer(h)
		server.Config.WriteTimeout = 50 * time.Millisecond
		server.Start()
		defer server.Close()
		for _, tc := range []struct{ path, model string }{{"/codex/responses", "approved-codex"}, {"/claude/v1/messages", "approved-claude"}} {
			req, _ := http.NewRequest("POST", server.URL+tc.path, strings.NewReader(`{"model":"`+tc.model+`","stream":true}`))
			req.Header.Set("Authorization", "Bearer workspace-grant")
			req.Header.Set("Anthropic-Beta", "oauth-capability,example-beta")
			client := &http.Client{Timeout: 2 * time.Second}
			res, err := client.Do(req)
			if err != nil {
				t.Fatal("native response was cut off by server deadline:", err)
			}
			body, err := io.ReadAll(res.Body)
			res.Body.Close()
			if err != nil || res.StatusCode != 200 || string(body) != "data: native-result\n\n" {
				t.Fatalf("incomplete native response: status=%d err=%v", res.StatusCode, err)
			}
		}
	})
	assertCatalog := func(want int) {
		r := httptest.NewRequest("GET", "/v1/models", nil)
		r.Header.Set("Authorization", "Bearer workspace-grant")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		var catalog struct {
			Items []map[string]string `json:"items"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &catalog) != nil || len(catalog.Items) != want || strings.Contains(w.Body.String(), "secret-marker") {
			t.Fatalf("catalog credential boundary: %d %s", w.Code, w.Body.String())
		}
	}
	assertCatalog(2)
	expiry.Store(time.Now().Add(-time.Second).Unix())
	assertCatalog(0)
	if w := request("/codex/responses", "workspace-grant", "approved-codex"); w.Code != 503 || !strings.Contains(w.Body.String(), "PROVIDER_LOGIN_REQUIRED") {
		t.Fatal("expired credential accepted")
	}
	if calls.Load() != 4 {
		t.Fatal("expired or denied request reached provider")
	}
}

func TestUserSubscriptionUsesVerifiedWorkerBinding(t *testing.T) {
	for _, provider := range []string{"codex", "claude"} {
		t.Run(provider, func(t *testing.T) {
			var available atomic.Bool
			available.Store(true)
			var calls atomic.Int32
			cfg := Config{Mode: "fixture", Models: []string{"fixture"}, Subscriptions: &SubscriptionConfig{UserAccounts: true, Providers: map[string]SubscriptionProvider{provider: {Models: []string{"test-model"}, Network: NetworkPolicy{AllowedCIDRs: []string{"203.0.113.7/32"}}}}}, Verify: func(_ context.Context, token, audience string) (Identity, error) {
				if token != "worker-grant" {
					return Identity{}, ErrDenied
				}
				return Identity{WorkspaceID: "ws_verified", ProjectID: "default", PrincipalID: "operator"}, nil
			}, ResolveSubscription: func(_ context.Context, workspace, selectedProvider, _ string) (subscriptionCredential, error) {
				if workspace != "ws_verified" || selectedProvider != provider {
					t.Error("unverified workspace")
				}
				if !available.Load() {
					return subscriptionCredential{}, ErrDenied
				}
				return subscriptionCredential{AccessToken: "private-access", AccountID: "selected-account", ExpiresAt: time.Now().Add(time.Hour).Unix()}, nil
			}}
			cfg.subscriptionHTTP = &http.Client{Transport: subscriptionRoundTrip(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				if r.Header.Get("Authorization") != "Bearer private-access" || (provider == "codex" && r.Header.Get("ChatGPT-Account-Id") != "selected-account") {
					t.Error("wrong account")
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("result"))}, nil
			})}
			h, err := NewHandler(cfg)
			if err != nil {
				t.Fatal(err)
			}
			call := func(token string) *httptest.ResponseRecorder {
				path := "/codex/responses"
				if provider == "claude" {
					path = "/claude/v1/messages"
				}
				r := httptest.NewRequest("POST", path, strings.NewReader(`{"model":"test-model"}`))
				r.Header.Set("Authorization", "Bearer "+token)
				r.Header.Set("X-Workbench-Subscription-User", "user_victim")
				r.Header.Set("ChatGPT-Account-Id", "victim-account")
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				return w
			}
			if w := call("worker-grant"); w.Code != 200 || strings.Contains(w.Body.String(), "private-access") {
				t.Fatal(w.Code, w.Body)
			}
			if w := call("invalid"); w.Code != 401 {
				t.Fatal(w.Code)
			}
			available.Store(false)
			if w := call("worker-grant"); w.Code != 503 {
				t.Fatal(w.Code)
			}
			if calls.Load() != 1 {
				t.Fatal("unloaded credential reached provider")
			}
		})
	}
}
