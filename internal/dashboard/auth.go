package dashboard

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"hash/maphash"
	"log/slog"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"agent-workbench/internal/shared"
	workos "github.com/workos/workos-go/v10"
	"golang.org/x/time/rate"
)

// AuthConfig gates login. Verified human identity is forwarded to the API,
// which enforces explicit project membership.
// With no configuration the existing loopback-only local mode is preserved.
type AuthConfig struct {
	ClientID, APIKey, RedirectURI string
	AllowedEmails                 []string
}

func AuthConfigFromEnv() (AuthConfig, error) {
	key, err := shared.Secret("WORKOS_API_KEY")
	return AuthConfig{
		ClientID: strings.TrimSpace(os.Getenv("WORKOS_CLIENT_ID")), APIKey: key,
		RedirectURI:   strings.TrimSpace(os.Getenv("WORKOS_REDIRECT_URI")),
		AllowedEmails: strings.FieldsFunc(os.Getenv("WORKOS_ALLOWED_EMAILS"), func(r rune) bool { return r == ',' || r == '\n' }),
	}, err
}

type authenticatedOriginKey struct{}
type authAttempt struct {
	State, Verifier string
	Expires         time.Time
}
type operatorSession struct {
	mu                sync.Mutex // Serializes refresh-token rotation and logout for this session.
	sealed, sessionID string
	userID, authState string
	expires           time.Time
	revoked           bool
}
type authHandler struct {
	next                   http.Handler
	client                 *workos.Client
	cfg                    AuthConfig
	origin, host, password string
	secure                 bool
	allowed                map[string]bool
	mu                     sync.Mutex
	attemptCipher          cipher.AEAD
	loginLimits            [256]*rate.Limiter
	sessions               map[string]*operatorSession
}

func NewAuthKitHandler(next http.Handler, cfg AuthConfig) (http.Handler, error) {
	if cfg.ClientID == "" && cfg.APIKey == "" && cfg.RedirectURI == "" && len(cfg.AllowedEmails) == 0 {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/auth/session" && r.Method == http.MethodGet {
				w.Header().Set("Cache-Control", "no-store")
				shared.JSON(w, 200, map[string]string{"status": "local"})
				return
			}
			next.ServeHTTP(w, withSubscriptionIdentity(r, "local", "operator"))
		}), nil
	}
	if cfg.RedirectURI == "" {
		cfg.RedirectURI = "http://127.0.0.1:8080/auth/callback"
	}
	u, err := url.Parse(cfg.RedirectURI)
	if err != nil || u.Host == "" || u.User != nil || u.Path != "/auth/callback" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1"))) {
		return nil, errors.New("WORKOS_REDIRECT_URI must be HTTPS (or HTTP loopback) ending in /auth/callback")
	}
	if cfg.ClientID == "" || cfg.APIKey == "" {
		return nil, errors.New("WorkOS requires both WORKOS_CLIENT_ID and WORKOS_API_KEY")
	}
	allowed := map[string]bool{}
	for _, email := range cfg.AllowedEmails {
		email = strings.TrimSpace(email)
		if email == "" {
			continue
		}
		parsed, err := mail.ParseAddress(email)
		if err != nil || parsed.Address != email {
			return nil, errors.New("WORKOS_ALLOWED_EMAILS must contain plain email addresses")
		}
		allowed[strings.ToLower(email)] = true
	}
	if len(allowed) == 0 {
		return nil, errors.New("WORKOS_ALLOWED_EMAILS is required for operator access")
	}
	client := workos.NewClient(cfg.APIKey, workos.WithClientID(cfg.ClientID), workos.WithMaxRetries(0), workos.WithHTTPClient(&http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}))
	password := rand.Text() + rand.Text()
	key := sha256.Sum256([]byte("authkit-pkce:" + password))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	attemptCipher, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return nil, err
	}
	handler := &authHandler{next: next, client: client, cfg: cfg, origin: u.Scheme + "://" + u.Host, host: u.Host, secure: u.Scheme == "https", password: password, allowed: allowed, attemptCipher: attemptCipher, sessions: map[string]*operatorSession{}}
	for i := range handler.loginLimits {
		handler.loginLimits[i] = rate.NewLimiter(8, 32)
	}
	return handler, nil
}

func (a *authHandler) cookie(w http.ResponseWriter, name, value string, age int) {
	c := &http.Cookie{Name: hostCookieName(name, a.secure), Value: value, Path: "/", MaxAge: age, HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode}
	if age < 0 {
		c.Expires = time.Unix(1, 0)
	}
	http.SetCookie(w, c)
}
func (a *authHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if a.secure && servePublicWorker(a.next, w, r, a.host) {
		return
	}
	if !a.secure && localWorkerHost(r) {
		// A loopback AuthKit login can still issue worker console and preview tickets.
		if strings.HasPrefix(requestHostname(r), "desktop-") {
			writeError(w, 404, "NOT_FOUND")
			return
		}
		a.next.ServeHTTP(w, r)
		return
	}
	if r.URL.Path == "/healthz" && r.Method == http.MethodGet {
		shared.JSON(w, 200, map[string]string{"status": "ok"})
		return
	}
	if !strings.EqualFold(r.Host, a.host) {
		// Local entry links may use either loopback name. Canonicalize only
		// navigation before issuing auth cookies; APIs and callbacks stay strict.
		canonical, _ := url.Parse(a.origin)
		incoming := &url.URL{Host: r.Host}
		loopback := func(host string) bool { return host == "localhost" || host == "127.0.0.1" }
		entry := r.URL.Path == "/" || r.URL.Path == "/app" || strings.HasPrefix(r.URL.Path, "/app/") || r.URL.Path == "/auth/login"
		if canonical.Scheme == "http" && loopback(canonical.Hostname()) && loopback(strings.ToLower(incoming.Hostname())) && incoming.Port() == canonical.Port() && entry && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			canonical.Path, canonical.RawPath, canonical.RawQuery = r.URL.Path, r.URL.RawPath, r.URL.RawQuery
			w.Header().Set("Cache-Control", "no-store")
			http.Redirect(w, r, canonical.String(), http.StatusTemporaryRedirect)
			return
		}
		writeError(w, 403, "HOST_REJECTED")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	if origin := r.Header.Get("Origin"); origin != "" && origin != a.origin {
		writeError(w, 403, "ORIGIN_REJECTED")
		return
	}
	r = r.WithContext(context.WithValue(r.Context(), authenticatedOriginKey{}, a.origin))
	switch r.URL.Path {
	case "/auth/login":
		if r.Method != http.MethodGet {
			writeError(w, 405, "METHOD_NOT_ALLOWED")
			return
		}
		a.login(w, r)
		return
	case "/auth/callback":
		if r.Method != http.MethodGet {
			writeError(w, 405, "METHOD_NOT_ALLOWED")
			return
		}
		a.callback(w, r)
		return
	case "/auth/logout":
		if r.Method != http.MethodPost {
			writeError(w, 405, "METHOD_NOT_ALLOWED")
			return
		}
		if r.Header.Get("Origin") != a.origin || r.Header.Get("X-Workbench-Request") != "1" {
			writeError(w, 403, "ORIGIN_REJECTED")
			return
		}
		a.logout(w, r)
		return
	}
	// The shell contains no private data. The React boundary displays sign-in
	// until session resolution succeeds; all data and mutations are gated below.
	if r.Method == http.MethodGet && (r.URL.Path == "/app" || strings.HasPrefix(r.URL.Path, "/app/") || r.URL.Path == "/") {
		a.next.ServeHTTP(w, r)
		return
	}
	result, err := a.authenticate(r)
	if err != nil {
		writeError(w, 503, "AUTH_UNAVAILABLE")
		return
	}
	if r.URL.Path == "/auth/session" {
		if r.Method != http.MethodGet {
			writeError(w, 405, "METHOD_NOT_ALLOWED")
			return
		}
		if result == nil {
			a.cookie(w, "awb_session", "", -1)
			shared.JSON(w, 200, map[string]string{"status": "anonymous"})
			return
		}
		user := result.User
		name := ""
		if user.FirstName != nil {
			name = *user.FirstName
		}
		if user.LastName != nil {
			name += " " + *user.LastName
		}
		name = strings.TrimSpace(name)
		if name == "" {
			name = user.Email
		}
		shared.JSON(w, 200, map[string]any{"status": "authenticated", "user": map[string]string{"id": user.ID, "name": name, "email": user.Email}})
		return
	}
	if result == nil {
		writeError(w, 401, "AUTH_REQUIRED")
		return
	}
	a.next.ServeHTTP(w, withHumanIdentity(r, "workos:"+a.cfg.ClientID, result.User.ID, result.User.Email))
}

var loginBucketSeed = maphash.MakeSeed()

func (a *authHandler) login(w http.ResponseWriter, r *http.Request) {
	// Use the transport peer, never a caller-controlled forwarded address.
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		peer = r.RemoteAddr
	}
	if !a.loginLimits[maphash.String(loginBucketSeed, peer)%uint64(len(a.loginLimits))].Allow() {
		writeError(w, 429, "RATE_LIMITED")
		return
	}
	provider := "authkit"
	auth, err := a.client.GetAuthKitPKCEAuthorizationURL(workos.AuthKitAuthorizationURLParams{RedirectURI: a.cfg.RedirectURI, Provider: &provider})
	if err != nil {
		writeError(w, 503, "AUTH_UNAVAILABLE")
		return
	}
	a.cookie(w, "awb_auth_state", a.sealAttempt(authAttempt{State: auth.State, Verifier: auth.CodeVerifier, Expires: time.Now().Add(10 * time.Minute)}), 600)
	http.Redirect(w, r, auth.URL, http.StatusFound)
}
func (a *authHandler) sealAttempt(attempt authAttempt) string {
	raw, _ := json.Marshal(attempt)
	return base64.RawURLEncoding.EncodeToString(a.attemptCipher.Seal(nil, nil, raw, []byte(a.origin)))
}

func (a *authHandler) callback(w http.ResponseWriter, r *http.Request) {
	cookie, err := uniqueCookie(r, hostCookieName("awb_auth_state", a.secure))
	state := r.URL.Query().Get("state")
	if err != nil || state == "" {
		a.failLogin(w, r, "invalid_state")
		return
	}
	a.cookie(w, "awb_auth_state", "", -1)
	attemptBlob, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil || len(attemptBlob) > 2048 {
		a.failLogin(w, r, "invalid_state")
		return
	}
	raw, err := a.attemptCipher.Open(nil, nil, attemptBlob, []byte(a.origin))
	var attempt authAttempt
	if err != nil || json.Unmarshal(raw, &attempt) != nil || !time.Now().Before(attempt.Expires) ||
		subtle.ConstantTimeCompare([]byte(state), []byte(attempt.State)) != 1 {
		a.failLogin(w, r, "invalid_state")
		return
	}
	a.mu.Lock()
	for _, session := range a.sessions {
		if session.authState == state {
			a.mu.Unlock()
			a.failLogin(w, r, "invalid_state")
			return
		}
	}
	a.mu.Unlock()
	code := r.URL.Query().Get("code")
	if code == "" || r.URL.Query().Get("error") != "" {
		logAuthFailure("provider_callback", nil, "")
		a.failLogin(w, r, "sign_in_failed")
		return
	}
	response, err := a.client.AuthKitPKCECodeExchange(r.Context(), workos.AuthKitPKCECodeExchangeParams{Code: code, CodeVerifier: attempt.Verifier})
	if err != nil {
		logAuthFailure("code_exchange", err, "")
		a.failLogin(w, r, "sign_in_failed")
		return
	}
	sealed, err := workos.SealSession(&workos.SessionData{AccessToken: response.AccessToken, RefreshToken: response.RefreshToken, User: response.User, Impersonator: response.Impersonator}, a.password)
	if err != nil {
		logAuthFailure("seal_session", err, "")
		a.failLogin(w, r, "sign_in_failed")
		return
	}
	result, err := a.newSession(sealed).AuthenticateContext(r.Context())
	if err != nil || result == nil || !result.Authenticated || result.SessionID == "" {
		reason := "missing_session"
		if result != nil {
			reason = result.Reason
		}
		logAuthFailure("validate_session", err, reason)
		a.failLogin(w, r, "sign_in_failed")
		return
	}
	if !a.permitted(result) {
		a.failLogin(w, r, "access_denied")
		return
	}
	key := rand.Text() + rand.Text()
	a.mu.Lock()
	now := time.Now()
	var oldestID string
	var oldest time.Time
	count := 0
	for id, session := range a.sessions {
		if !now.Before(session.expires) {
			delete(a.sessions, id)
		} else if session.userID == result.User.ID {
			count++
			if oldestID == "" || session.expires.Before(oldest) {
				oldestID, oldest = id, session.expires
			}
		}
	}
	if old, err := uniqueCookie(r, hostCookieName("awb_session", a.secure)); err == nil {
		if session := a.sessions[old.Value]; session != nil && session.userID == result.User.ID {
			delete(a.sessions, old.Value)
			count--
		}
	}
	if count >= 8 {
		delete(a.sessions, oldestID)
	}
	if len(a.sessions) >= 1024 {
		a.mu.Unlock()
		a.failLogin(w, r, "sign_in_failed")
		return
	}
	a.sessions[key] = &operatorSession{sealed: sealed, sessionID: result.SessionID, userID: result.User.ID, authState: state, expires: now.Add(7 * 24 * time.Hour)}
	a.mu.Unlock()
	a.cookie(w, "awb_session", key, 7*24*60*60)
	http.Redirect(w, r, "/app/", http.StatusSeeOther)
}

// Never log the error text or raw WorkOS response: they may include tokens.
func logAuthFailure(stage string, err error, reason string) {
	fields := []any{"stage", stage, "reason", reason}
	var apiErr *workos.APIError
	if errors.As(err, &apiErr) {
		fields = append(fields, "status", apiErr.StatusCode, "code", apiErr.Code, "oauth_error", apiErr.ErrorCode, "request_id", apiErr.RequestID)
	}
	slog.Warn("dashboard sign-in failed", fields...)
}
func (a *authHandler) failLogin(w http.ResponseWriter, r *http.Request, code string) {
	http.Redirect(w, r, "/app/?auth_error="+code, http.StatusSeeOther)
}
func (a *authHandler) permitted(result *workos.AuthenticateSessionResult) bool {
	return result.Authenticated && result.User != nil && result.User.ID != "" && result.User.EmailVerified && a.allowed[strings.ToLower(result.User.Email)] && result.Impersonator == nil
}
func (a *authHandler) lookup(r *http.Request) *operatorSession {
	cookie, err := uniqueCookie(r, hostCookieName("awb_session", a.secure))
	if err != nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	session := a.sessions[cookie.Value]
	if session != nil && time.Now().After(session.expires) {
		delete(a.sessions, cookie.Value)
		return nil
	}
	return session
}

// Pin the issuer to the configured application, never to an unverified token.
// WorkOS publishes this issuer in its client-specific OIDC discovery metadata.
func (a *authHandler) newSession(sealed string) *workos.Session {
	issuer := "https://api.workos.com/user_management/" + a.cfg.ClientID
	return workos.NewSession(a.client, sealed, a.password, workos.WithSessionIssuer(issuer))
}

func (a *authHandler) authenticate(r *http.Request) (*workos.AuthenticateSessionResult, error) {
	entry := a.lookup(r)
	if entry == nil {
		return nil, nil
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.revoked {
		return nil, nil
	}
	defer func() {
		if entry.revoked {
			revokeWorkerSessions(a.next, "workos:"+a.cfg.ClientID, entry.userID)
		}
	}()
	session := a.newSession(entry.sealed)
	result, err := session.AuthenticateContext(r.Context())
	if err != nil {
		return nil, err
	} // Preserve session on transient JWKS failures.
	if result.NeedsRefresh {
		refreshed, err := session.Refresh(r.Context())
		if refreshed != nil && refreshed.Reason == "refresh_token_revoked" {
			entry.revoked = true
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if refreshed == nil || !refreshed.Authenticated {
			entry.revoked = true
			return nil, nil
		}
		entry.sealed = refreshed.SealedSession
		result, err = a.newSession(entry.sealed).AuthenticateContext(r.Context())
		if err != nil {
			return nil, err
		}
	}
	if !a.permitted(result) || result.SessionID != entry.sessionID {
		entry.revoked = true
		return nil, nil
	}
	return result, nil
}
func (a *authHandler) logout(w http.ResponseWriter, r *http.Request) {
	returnTo := a.origin + "/app/"
	logoutURL := returnTo
	if entry := a.lookup(r); entry != nil {
		entry.mu.Lock()
		// Revoke locally first, even if the browser cannot reach WorkOS afterwards.
		entry.revoked = true
		logoutURL = a.client.UserManagement().GetLogoutURL(&workos.UserManagementGetLogoutURLParams{SessionID: entry.sessionID, ReturnTo: &returnTo})
		revokeWorkerSessions(a.next, "workos:"+a.cfg.ClientID, entry.userID)
		entry.mu.Unlock()
	}
	if cookie, err := uniqueCookie(r, hostCookieName("awb_session", a.secure)); err == nil {
		a.mu.Lock()
		delete(a.sessions, cookie.Value)
		a.mu.Unlock()
	}
	a.cookie(w, "awb_session", "", -1)
	shared.JSON(w, 200, map[string]string{"url": logoutURL})
}
