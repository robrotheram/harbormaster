package gateway

import (
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestModelCatalogUsesWorkspaceAuthorization(t *testing.T) {
	c := Config{Mode: "fixture", Models: []string{"fixture"}, Subscriptions: &SubscriptionConfig{UserAccounts: true, Providers: map[string]SubscriptionProvider{"codex": {Models: []string{"configured-model"}, Network: NetworkPolicy{AllowedCIDRs: []string{"203.0.113.7/32"}}}, "claude": {Models: []string{"unavailable-model"}, Network: NetworkPolicy{AllowedCIDRs: []string{"203.0.113.8/32"}}}}}, Verify: func(_ context.Context, token, audience string) (Identity, error) {
		if token != "workspace-grant" || audience != "inference" {
			return Identity{}, ErrDenied
		}
		return Identity{PrincipalID: "alice", ProjectID: "default", WorkspaceID: "ws_one"}, nil
	}, ResolveSubscription: func(_ context.Context, workspace, provider string) (subscriptionCredential, error) {
		if workspace != "ws_one" {
			t.Fatalf("wrong workspace: %s", workspace)
		}
		expiry := time.Now().Add(time.Hour).Unix()
		if provider == "claude" {
			expiry = time.Now().Add(-time.Hour).Unix()
		}
		return subscriptionCredential{AccessToken: "secret-never-exposed", ExpiresAt: expiry}, nil
	}}
	h, err := NewHandler(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"invalid", "workspace-grant"} {
		r := httptest.NewRequest("GET", "/v1/models", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if token == "invalid" {
			if w.Code != 401 {
				t.Fatal(w.Code)
			}
			continue
		}
		var catalog shared.AgentModels
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &catalog) != nil {
			t.Fatal(w.Code, w.Body.String())
		}
		if len(catalog.Items) != 1 || catalog.Items[0].Model != "configured-model" || catalog.Items[0].Provider != "codex" {
			t.Fatal(catalog)
		}
		if strings.Contains(w.Body.String(), "secret") || !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
			t.Fatal("credential/cache boundary")
		}
	}
}
