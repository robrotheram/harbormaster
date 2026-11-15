package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUserSubscriptionLifecycle(t *testing.T) {
	s, _ := setup(t)
	s.cfg.SubscriptionGatewayToken = "gateway-only"
	ws := request(t, s, "POST", "/v1/workspaces", `{"idempotency_key":"sub-worker"}`)
	var worker struct {
		ID string `json:"id"`
	}
	json.Unmarshal(ws.Body.Bytes(), &worker)
	if worker.ID == "" {
		t.Fatal(ws.Body)
	}
	call := func(method, path, user, token, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set(subscriptionUserHeader, user)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w
	}
	s.cfg.ProjectTokens["project-only"] = "default"
	if res := call("GET", "/v1/subscription", "user_alice", "project-only", ""); res.Code != 403 {
		t.Fatal("project credential accessed personal subscription", res.Code)
	}
	claims := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d,"https://api.openai.com/auth":{"chatgpt_account_id":"account"}}`, time.Now().Add(time.Hour).Unix())))
	body := `{"auth":{"auth_mode":"chatgpt","tokens":{"access_token":"header.` + claims + `.signature","account_id":"account","refresh_token":"discard-secret"}}}`
	for _, user := range []string{"user_alice", "user_bob"} {
		if res := call("POST", "/v1/subscription", user, s.cfg.Token, body); res.Code != 200 {
			t.Fatal(res.Code, res.Body)
		}
	}
	sealed, err := s.Store.Subscription(context.Background(), "user_alice")
	if err != nil || strings.Contains(sealed, "signature") || strings.Contains(sealed, "discard-secret") {
		t.Fatal("credential persistence")
	}
	res := call("GET", "/v1/subscription", "user_stranger", s.cfg.Token, "")
	if res.Code != 200 || !strings.Contains(res.Body.String(), `"saved":false`) {
		t.Fatal("cross-user read", res.Body)
	}
	if res := call("GET", "/v1/subscription", "", s.cfg.Token, ""); res.Code != 401 {
		t.Fatal("missing identity allowed")
	}
	path := "/v1/workspaces/" + worker.ID + "/subscription/"
	if res := call("POST", path+"load", "user_alice", s.cfg.Token, "{}"); res.Code != 200 {
		t.Fatal(res.Code, res.Body)
	}
	if res := call("POST", path+"load", "user_bob", s.cfg.Token, "{}"); res.Code != 409 {
		t.Fatal("replacement accepted", res.Code, res.Body)
	}
	resolve := func(token string) *httptest.ResponseRecorder {
		return call("POST", "/internal/subscriptions/resolve", "", token, `{"workspace_id":"`+worker.ID+`"}`)
	}
	if resolve(s.cfg.Token).Code != 401 {
		t.Fatal("management token can extract credentials")
	}
	if resolve("gateway-only").Code != 200 {
		t.Fatal("gateway cannot resolve")
	}
	if res := call("POST", path+"unload", "user_bob", s.cfg.Token, "{}"); res.Code != 200 {
		t.Fatal(res.Body)
	}
	if resolve("gateway-only").Code != 200 {
		t.Fatal("other user unloaded binding")
	}
	if res := call("POST", "/v1/subscription/delete", "user_alice", s.cfg.Token, "{}"); res.Code != 200 {
		t.Fatal(res.Body)
	}
	if resolve("gateway-only").Code != 503 {
		t.Fatal("deleted subscription still usable")
	}
	if res := call("POST", path+"load", "user_bob", s.cfg.Token, "{}"); res.Code != 200 {
		t.Fatal(res.Body)
	}
}

func TestMultipleSubscriptionsArePrivateAndProviderScoped(t *testing.T) {
	s, _ := setup(t)
	s.cfg.SubscriptionGatewayToken = "gateway-only"
	var worker struct {
		ID string `json:"id"`
	}
	json.Unmarshal(request(t, s, "POST", "/v1/workspaces", `{"idempotency_key":"multi-sub"}`).Body.Bytes(), &worker)
	call := func(method, path, user, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+s.cfg.Token)
		if strings.HasPrefix(path, "/internal/") {
			r.Header.Set("Authorization", "Bearer gateway-only")
		}
		r.Header.Set(subscriptionUserHeader, user)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w
	}
	expiry := time.Now().Add(time.Hour)
	claims := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d,"https://api.openai.com/auth":{"chatgpt_account_id":"account"}}`, expiry.Unix())))
	codex := `{"auth_mode":"chatgpt","tokens":{"access_token":"header.` + claims + `.signature","account_id":"account"}}`
	claude := fmt.Sprintf(`{"claudeAiOauth":{"accessToken":"sk-ant-oat-test-secret","refreshToken":"discard-me","expiresAt":%d,"scopes":["user:inference"]}}`, expiry.UnixMilli())
	create := func(user, name, provider, auth string) string {
		t.Helper()
		res := call("POST", "/v1/subscriptions", user, fmt.Sprintf(`{"name":%q,"provider":%q,"auth":%s}`, name, provider, auth))
		if res.Code != 200 {
			t.Fatal(res.Code, res.Body)
		}
		var v struct{ ID string }
		json.Unmarshal(res.Body.Bytes(), &v)
		return v.ID
	}
	a := create("user_alice", "Personal", "codex", codex)
	b := create("user_alice", "Work", "codex", codex)
	c := create("user_alice", "Claude", "claude", claude)
	d := create("user_alice", "Claude Work", "claude", claude)
	other := create("user_bob", "Other", "codex", codex)
	for _, id := range []string{a, b, c, d} {
		res := call("POST", "/v1/subscriptions/"+id, "user_bob", `{"name":"stolen"}`)
		if res.Code != 404 {
			t.Fatal("cross-owner update", res.Code)
		}
		if res = call("POST", "/v1/subscriptions/"+id+"/delete", "user_bob", "{}"); res.Code != 404 {
			t.Fatal("cross-owner delete", res.Code)
		}
	}
	path := "/v1/workspaces/" + worker.ID + "/subscription/"
	load := func(user, id string) int { return call("POST", path+"load", user, `{"subscription_id":"`+id+`"}`).Code }
	if load("user_alice", a) != 200 || load("user_alice", c) != 200 || load("user_alice", b) != 200 || load("user_alice", d) != 200 {
		t.Fatal("load or switch failed")
	}
	if load("user_bob", a) != 409 || load("user_bob", other) != 409 {
		t.Fatal("another user took over")
	}
	resolve := func(provider string) *httptest.ResponseRecorder {
		return call("POST", "/internal/subscriptions/resolve", "", fmt.Sprintf(`{"workspace_id":%q,"provider":%q}`, worker.ID, provider))
	}
	if resolve("codex").Code != 200 || resolve("claude").Code != 200 {
		t.Fatal("provider resolution failed")
	}
	res := call("GET", "/v1/subscriptions", "user_alice", "")
	if res.Code != 200 || strings.Contains(res.Body.String(), "secret") || strings.Contains(res.Body.String(), "signature") || strings.Contains(res.Body.String(), "discard-me") {
		t.Fatal("credential leaked", res.Code)
	}
	var listed struct {
		Items []struct {
			ID      string
			Workers []string
		}
	}
	json.Unmarshal(res.Body.Bytes(), &listed)
	if len(listed.Items) != 4 {
		t.Fatal("missing subscriptions")
	}
	for _, v := range listed.Items {
		want := v.ID == b || v.ID == d
		if (len(v.Workers) == 1) != want {
			t.Fatal("wrong binding", v.ID)
		}
	}
	if res := call("POST", "/v1/subscriptions/"+b, "user_alice", `{"name":"Renamed"}`); res.Code != 200 {
		t.Fatal("rename", res.Code)
	}
	if res := call("POST", "/v1/subscriptions/"+b, "user_alice", `{"name":"Changed","provider":"claude"}`); res.Code != 400 {
		t.Fatal("provider changed")
	}
	if res := call("POST", "/v1/subscriptions/"+a+"/delete", "user_alice", "{}"); res.Code != 200 {
		t.Fatal("delete unused")
	}
	if resolve("codex").Code != 200 {
		t.Fatal("deleting unused account removed active one")
	}
	if res := call("POST", path+"unload", "user_alice", `{"provider":"claude"}`); res.Code != 200 {
		t.Fatal("unload")
	}
	if resolve("claude").Code != 503 || resolve("codex").Code != 200 {
		t.Fatal("unload not provider scoped")
	}
	if res := call("POST", "/v1/subscriptions/"+b+"/delete", "user_alice", "{}"); res.Code != 200 || resolve("codex").Code != 503 {
		t.Fatal("delete did not revoke")
	}
	if load("user_bob", other) != 200 {
		t.Fatal("binding not released")
	}
}
