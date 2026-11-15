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
