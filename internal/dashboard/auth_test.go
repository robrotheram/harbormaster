package dashboard

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	workos "github.com/workos/workos-go/v10"
)

func authRequest(h http.Handler, method, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://127.0.0.1:8080"+path, nil)
	if cookie != nil {
		r.AddCookie(cookie)
	}
	if method == "POST" {
		r.Header.Set("Origin", "http://127.0.0.1:8080")
		r.Header.Set("X-Workbench-Request", "1")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func authFixture(t *testing.T) (*authHandler, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	exchanges, refreshes := &atomic.Int32{}, &atomic.Int32{}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/sso/jwks/client_test" {
			json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{"kid": "test", "kty": "RSA", "alg": "RS256", "use": "sig", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())}}})
			return
		}
		if r.URL.Path != "/user_management/authenticate" {
			http.NotFound(w, r)
			return
		}
		var input map[string]string
		json.NewDecoder(r.Body).Decode(&input)
		code := input["code"]
		if input["grant_type"] == "refresh_token" {
			refreshes.Add(1)
			if input["refresh_token"] == "transient" {
				w.WriteHeader(503)
				w.Write([]byte(`{"message":"unavailable"}`))
				return
			}
			if input["refresh_token"] == "revoked" {
				w.WriteHeader(400)
				w.Write([]byte(`{"error":"invalid_grant","error_description":"revoked"}`))
				return
			}
		} else {
			exchanges.Add(1)
			if input["code_verifier"] == "" || input["client_id"] != "client_test" || input["client_secret"] != "sk_test" {
				t.Error("missing PKCE or client credentials")
			}
		}
		email := "operator@example.com"
		if code == "denied" {
			email = "other@example.com"
		}
		expiry := time.Now().Add(time.Hour)
		if code == "expired" {
			expiry = time.Now().Add(-time.Minute)
		}
		claims := map[string]any{"iss": "https://api.workos.com/user_management/client_test", "sub": "user_one", "sid": "session_one", "exp": expiry.Unix(), "auth_time": time.Now().Unix()}
		if code == "wrong-issuer" {
			claims["iss"] = "https://attacker.example"
		}
		if code == "wrong-client-issuer" {
			claims["iss"] = "https://api.workos.com/user_management/client_other"
		}
		if code == "legacy-issuer" {
			claims["iss"] = "https://api.workos.com/"
		}
		token := signAuthToken(t, key, claims)
		if code == "tampered" {
			token += "bad"
		}
		json.NewEncoder(w).Encode(map[string]any{"access_token": token, "refresh_token": "refresh_one", "user": map[string]any{"id": "user_one", "email": email, "email_verified": code != "unverified", "first_name": "Test", "last_name": "Operator"}})
	}))
	t.Cleanup(provider.Close)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	h, err := NewAuthKitHandler(next, AuthConfig{ClientID: "client_test", APIKey: "sk_test", AllowedEmails: []string{"operator@example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	a := h.(*authHandler)
	a.client = workos.NewClient("sk_test", workos.WithClientID("client_test"), workos.WithBaseURL(provider.URL), workos.WithMaxRetries(0))
	return a, exchanges, refreshes
}
func signAuthToken(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "test"})
	body, _ := json.Marshal(claims)
	data := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(body)
	digest := sha256.Sum256([]byte(data))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return data + "." + base64.RawURLEncoding.EncodeToString(signature)
}
func startAuth(t *testing.T, a *authHandler) (*http.Cookie, string) {
	t.Helper()
	w := authRequest(a, "GET", "/auth/login", nil)
	if w.Code != 302 {
		t.Fatal(w.Code)
	}
	redirect, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	q := redirect.Query()
	if q.Get("provider") != "authkit" || q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" || q.Get("redirect_uri") != "http://127.0.0.1:8080/auth/callback" {
		t.Fatal("invalid authorization URL")
	}
	return w.Result().Cookies()[0], q.Get("state")
}
func loginAuth(t *testing.T, a *authHandler) *http.Cookie {
	t.Helper()
	cookie, state := startAuth(t, a)
	w := authRequest(a, "GET", "/auth/callback?code=valid&state="+state, cookie)
	if w.Header().Get("Location") != "/app/" {
		t.Fatal("login failed", w.Header().Get("Location"))
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == "awb_session" {
			return c
		}
	}
	t.Fatal("no session")
	return nil
}
func TestAuthKitConfiguration(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	h, err := NewAuthKitHandler(next, AuthConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if w := authRequest(h, "GET", "/auth/session", nil); w.Code != 200 || !strings.Contains(w.Body.String(), `"local"`) {
		t.Fatal("local mode")
	}
	for _, cfg := range []AuthConfig{
		{ClientID: "client"}, {APIKey: "key"}, {ClientID: "client", APIKey: "key"},
		{ClientID: "client", APIKey: "key", AllowedEmails: []string{"*"}},
		{ClientID: "client", APIKey: "key", AllowedEmails: []string{"a@b.com"}, RedirectURI: "http://example.com/auth/callback"},
		{ClientID: "client", APIKey: "key", AllowedEmails: []string{"a@b.com"}, RedirectURI: "https://example.com/auth/callback?next=evil"},
	} {
		if _, err := NewAuthKitHandler(next, cfg); err == nil {
			t.Fatal("accepted incomplete/unsafe config")
		}
	}
}
func TestAuthKitSessionAndLogout(t *testing.T) {
	a, _, _ := authFixture(t)
	for _, path := range []string{"/ui-api/projects", "/dashboard-api/v1/snapshot", "/workspaces"} {
		if w := authRequest(a, "GET", path, nil); w.Code != 401 {
			t.Fatalf("ungated %s: %d", path, w.Code)
		}
	}
	if w := authRequest(a, "GET", "/app/", nil); w.Code != 204 {
		t.Fatal("shell unavailable")
	}
	cookie := loginAuth(t, a)
	if !cookie.HttpOnly || cookie.Domain != "" || cookie.SameSite != http.SameSiteLaxMode || len(cookie.Value) < 40 {
		t.Fatal("unsafe cookie")
	}
	w := authRequest(a, "GET", "/auth/session", cookie)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "operator@example.com") || strings.Contains(w.Body.String(), "refresh_one") || strings.Contains(w.Body.String(), "access_token") {
		t.Fatal("profile response", w.Code)
	}
	if w := authRequest(a, "GET", "/ui-api/projects", cookie); w.Code != 204 {
		t.Fatal("authorized request rejected")
	}
	if w := authRequest(a, "GET", "/auth/logout", cookie); w.Code != 405 {
		t.Fatal("GET logout accepted")
	}
	r := httptest.NewRequest("POST", "http://127.0.0.1:8080/auth/logout", nil)
	r.AddCookie(cookie)
	r.Header.Set("Origin", "https://evil.example")
	w = httptest.NewRecorder()
	a.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("cross-site logout")
	}
	w = authRequest(a, "POST", "/auth/logout", cookie)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "session_id=session_one") {
		t.Fatal("logout redirect missing")
	}
	if w := authRequest(a, "GET", "/ui-api/projects", cookie); w.Code != 401 {
		t.Fatal("copied cookie survived logout")
	}
}
func TestAuthKitRejectsCallbackAttacks(t *testing.T) {
	a, exchanges, _ := authFixture(t)
	cookie, state := startAuth(t, a)
	for _, c := range []*http.Cookie{nil, {Name: "awb_auth_state", Value: "different"}} {
		w := authRequest(a, "GET", "/auth/callback?code=valid&state="+state, c)
		if !strings.Contains(w.Header().Get("Location"), "invalid_state") {
			t.Fatal("missing state protection")
		}
	}
	if exchanges.Load() != 0 {
		t.Fatal("exchanged invalid state")
	}
	authRequest(a, "GET", "/auth/callback?code=valid&state="+state, cookie)
	authRequest(a, "GET", "/auth/callback?code=valid&state="+state, cookie)
	if exchanges.Load() != 1 {
		t.Fatal("callback replay exchanged code")
	}
	for _, code := range []string{"denied", "unverified", "expired", "wrong-issuer", "wrong-client-issuer", "legacy-issuer", "tampered"} {
		cookie, state := startAuth(t, a)
		w := authRequest(a, "GET", "/auth/callback?code="+code+"&state="+state, cookie)
		if !strings.Contains(w.Header().Get("Location"), "auth_error=") {
			t.Fatal("accepted", code)
		}
		for _, cookie := range w.Result().Cookies() {
			if cookie.Name == "awb_session" && cookie.MaxAge > 0 {
				t.Fatal("issued denied session")
			}
		}
	}
	cookie, state = startAuth(t, a)
	a.attempts[state] = authAttempt{verifier: "expired", expires: time.Now().Add(-time.Minute)}
	if w := authRequest(a, "GET", "/auth/callback?code=valid&state="+state, cookie); !strings.Contains(w.Header().Get("Location"), "invalid_state") {
		t.Fatal("expired callback")
	}
}
func TestAuthKitRefreshAndExpiry(t *testing.T) {
	a, _, refreshes := authFixture(t)
	cookie := loginAuth(t, a)
	entry := a.sessions[cookie.Value]
	// Use an actual signed, expired token from the fixture's code exchange.
	expired, err := a.client.AuthKitPKCECodeExchange(t.Context(), workos.AuthKitPKCECodeExchangeParams{Code: "expired", CodeVerifier: "test"})
	if err != nil {
		t.Fatal(err)
	}
	setExpired := func(refresh string) {
		entry.revoked = false
		entry.sealed, err = workos.SealSession(&workos.SessionData{AccessToken: expired.AccessToken, RefreshToken: refresh, User: expired.User}, a.password)
		if err != nil {
			t.Fatal(err)
		}
	}
	setExpired("refresh_one")
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if w := authRequest(a, "GET", "/ui-api/projects", cookie); w.Code != 204 {
				t.Errorf("refresh request: %d", w.Code)
			}
		})
	}
	wg.Wait()
	if refreshes.Load() != 1 {
		t.Fatal("parallel refresh rotated more than once", refreshes.Load())
	}
	setExpired("transient")
	if w := authRequest(a, "GET", "/auth/session", cookie); w.Code != 503 || entry.revoked {
		t.Fatal("transient failure destroyed session")
	}
	setExpired("revoked")
	if w := authRequest(a, "GET", "/ui-api/projects", cookie); w.Code != 401 || !entry.revoked {
		t.Fatal("revoked refresh accepted", w.Code)
	}
	entry.expires = time.Now().Add(-time.Second)
	if w := authRequest(a, "GET", "/ui-api/projects", cookie); w.Code != 401 {
		t.Fatal("expired local session")
	}
}
func TestAuthKitHostBoundary(t *testing.T) {
	a, _, _ := authFixture(t)
	for _, host := range []string{"localhost:8080", "evil.example", "evil.localhost:8080"} {
		r := httptest.NewRequest("GET", "http://"+host+"/ui-api/projects", nil)
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("host bypass", host, w.Code)
		}
	}
	a.secure = true
	w := authRequest(a, "GET", "/auth/login", nil)
	if !w.Result().Cookies()[0].Secure {
		t.Fatal("HTTPS cookie not Secure")
	}
}

func TestAuthKitLoopbackEntryRedirect(t *testing.T) {
	a, _, _ := authFixture(t)
	for _, tc := range []struct {
		method, host, path string
		status             int
	}{
		{"GET", "localhost:8080", "/", 307},
		{"HEAD", "localhost:8080", "/app/", 307},
		{"GET", "localhost:8080", "/app/projects?tab=active", 307},
		{"GET", "localhost:8080", "/auth/login", 307},
		{"POST", "localhost:8080", "/app/", 403},
		{"GET", "localhost:8080", "/auth/callback?code=secret", 403},
		{"GET", "localhost:8080", "/auth/session", 403},
		{"GET", "localhost:8080", "/ui-api/projects", 403},
		{"GET", "localhost:8081", "/", 403},
		{"GET", "evil.example:8080", "/", 403},
		{"GET", "evil.localhost:8080", "/", 403},
	} {
		t.Run(tc.method+tc.host+tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			a.ServeHTTP(w, httptest.NewRequest(tc.method, "http://"+tc.host+tc.path, nil))
			if w.Code != tc.status {
				t.Fatalf("status %d, want %d", w.Code, tc.status)
			}
			if tc.status == 307 && w.Header().Get("Location") != "http://127.0.0.1:8080"+tc.path {
				t.Fatalf("unexpected redirect: %s", w.Header().Get("Location"))
			}
			if len(w.Result().Cookies()) != 0 {
				t.Fatal("alias request issued cookies")
			}
		})
	}
	a.origin, a.host = "https://127.0.0.1:8080", "127.0.0.1:8080"
	w := httptest.NewRecorder()
	a.ServeHTTP(w, httptest.NewRequest("GET", "http://localhost:8080/", nil))
	if w.Code != 403 {
		t.Fatal("HTTPS origin accepted loopback alias")
	}
}
