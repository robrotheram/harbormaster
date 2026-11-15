package gateway

// Native CLI requests retain their provider protocol. Subscription credentials
// never cross this boundary into a workspace; its bearer is a workspace grant.
import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type SubscriptionProvider struct {
	Models  []string      `json:"models"`
	Network NetworkPolicy `json:"network"`
}
type SubscriptionConfig struct {
	UserAccounts bool                            `json:"user_accounts,omitempty"`
	Providers    map[string]SubscriptionProvider `json:"providers"`
	Principals   map[string][]string             `json:"principals"`
}
type subscriptionCredential struct {
	AccessToken string `json:"access_token"`
	AccountID   string `json:"account_id,omitempty"`
	ExpiresAt   int64  `json:"expires_at"`
}

func subscriptionVaultClient(address, caFile string) (*http.Client, error) {
	u, err := HTTPSOrigin(address)
	if err != nil || caFile == "" || u == nil {
		return nil, ErrDenied
	}
	b, err := os.ReadFile(caFile)
	if err != nil {
		return nil, ErrDenied
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(b) {
		return nil, ErrDenied
	}
	return &http.Client{Timeout: 5 * time.Second, CheckRedirect: NoRedirect, Transport: &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool}}}, nil
}

func registerSubscriptions(mux *http.ServeMux, c Config) error {
	if c.Subscriptions == nil {
		return nil
	}
	if len(c.Subscriptions.Providers) == 0 || (!c.Subscriptions.UserAccounts && (len(c.Subscriptions.Principals) == 0 || c.SubscriptionVaultToken == "")) || (c.Subscriptions.UserAccounts && c.ResolveSubscription == nil) {
		return ErrDenied
	}
	var vc *http.Client
	var err error
	if !c.Subscriptions.UserAccounts {
		vc, err = subscriptionVaultClient(c.SubscriptionVaultAddr, c.SubscriptionVaultCA)
		if err != nil {
			return err
		}
	}
	var renewalMu sync.Mutex
	var renewedAt time.Time
	renew := func(ctx context.Context) error {
		renewalMu.Lock()
		defer renewalMu.Unlock()
		if time.Since(renewedAt) < time.Hour {
			return nil
		}
		req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(c.SubscriptionVaultAddr, "/")+"/v1/auth/token/renew-self", strings.NewReader(`{}`))
		if err != nil {
			return ErrDenied
		}
		req.Header.Set("X-Vault-Token", c.SubscriptionVaultToken)
		res, err := vc.Do(req)
		if err != nil {
			return ErrDenied
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return ErrDenied
		}
		renewedAt = time.Now()
		return nil
	}
	for principal, providers := range c.Subscriptions.Principals {
		if !safeSubscriptionPart(principal) {
			return ErrDenied
		}
		for _, provider := range providers {
			if _, ok := c.Subscriptions.Providers[provider]; !ok {
				return ErrDenied
			}
		}
	}
	for provider, policy := range c.Subscriptions.Providers {
		origin := ""
		switch provider {
		case "codex":
			origin = "https://chatgpt.com/backend-api/codex"
		case "claude":
			origin = "https://api.anthropic.com"
		default:
			return ErrDenied
		}
		if len(policy.Models) == 0 {
			return ErrDenied
		}
		models := map[string]bool{}
		for _, model := range policy.Models {
			if model == "" {
				return ErrDenied
			}
			models[model] = true
		}
		tr, err := policy.Network.Transport(nil)
		if err != nil {
			return err
		}
		client := &http.Client{Transport: tr, Timeout: 10 * time.Minute, CheckRedirect: NoRedirect}
		if c.subscriptionHTTP != nil {
			client = c.subscriptionHTTP
		}
		handler := func(w http.ResponseWriter, r *http.Request) {
			id, err := Authenticate(r, c.Verify, "inference")
			if err != nil {
				Error(w, 401, "UNAUTHORIZED")
				return
			}
			allowed := c.Subscriptions.UserAccounts
			for _, p := range c.Subscriptions.Principals[id.PrincipalID] {
				if p == provider {
					allowed = true
				}
			}
			if !allowed || !safeSubscriptionPart(id.PrincipalID) {
				Error(w, 403, "NOT_FOUND_OR_FORBIDDEN")
				return
			}
			query := r.URL.Query()
			for k, v := range query {
				if provider != "claude" || k != "beta" || len(v) != 1 || v[0] != "true" {
					Error(w, 400, "INVALID_REQUEST")
					return
				}
			}
			body, err := ReadBounded(r.Body, 4<<20)
			var fields struct {
				Model string `json:"model"`
			}
			if err != nil || json.Unmarshal(body, &fields) != nil || !models[fields.Model] {
				Error(w, 400, "MODEL_NOT_ALLOWED")
				return
			}
			// The path is selected by a registered handler; no caller URL, Vault
			// path, principal, authorization or identity header is forwarded.
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
			defer cancel()
			var cred subscriptionCredential
			if c.Subscriptions.UserAccounts {
				var resolveErr error
				cred, resolveErr = c.ResolveSubscription(ctx, id.WorkspaceID, provider)
				if resolveErr != nil {
					Error(w, 503, "PROVIDER_LOGIN_REQUIRED")
					return
				}
			} else {
				if renew(ctx) != nil {
					Error(w, 503, "CREDENTIAL_UNAVAILABLE")
					return
				}
				vreq, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(c.SubscriptionVaultAddr, "/")+"/v1/subscription-auth/data/"+id.PrincipalID+"/"+provider, nil)
				if err != nil {
					Error(w, 503, "CREDENTIAL_UNAVAILABLE")
					return
				}
				vreq.Header.Set("X-Vault-Token", c.SubscriptionVaultToken)
				vres, err := vc.Do(vreq)
				if err != nil {
					Error(w, 503, "CREDENTIAL_UNAVAILABLE")
					return
				}
				vb, verr := ReadBounded(vres.Body, 64<<10)
				vres.Body.Close()
				var secret struct {
					Data struct {
						Data subscriptionCredential `json:"data"`
					} `json:"data"`
				}
				if verr != nil || vres.StatusCode != 200 || json.Unmarshal(vb, &secret) != nil {
					Error(w, 503, "PROVIDER_LOGIN_REQUIRED")
					return
				}
				cred = secret.Data.Data
			}
			if cred.AccessToken == "" || strings.ContainsAny(cred.AccessToken, " \r\n\t") || cred.ExpiresAt <= time.Now().Add(30*time.Second).Unix() {
				Error(w, 503, "PROVIDER_LOGIN_REQUIRED")
				return
			}
			upstreamCtx, stop := context.WithDeadline(ctx, time.Unix(cred.ExpiresAt, 0))
			defer stop()
			// Native generations may exceed the server's short default write
			// timeout. Bound this response by its request/credential lifetime,
			// otherwise a healthy long generation is truncated during streaming.
			deadline, _ := upstreamCtx.Deadline()
			if err := http.NewResponseController(w).SetWriteDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
				Error(w, 503, "UPSTREAM_UNAVAILABLE")
				return
			}
			// A stopped/revoked workspace loses an already-open stream as well.
			go func() {
				ticker := time.NewTicker(5 * time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-upstreamCtx.Done():
						return
					case <-ticker.C:
						if c.Subscriptions.UserAccounts {
							current, err := c.ResolveSubscription(upstreamCtx, id.WorkspaceID, provider)
							if err != nil || current.AccessToken != cred.AccessToken || current.AccountID != cred.AccountID {
								stop()
								return
							}
						}
						if _, err := c.Verify(upstreamCtx, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), "inference"); err != nil {
							stop()
							return
						}
					}
				}
			}()
			path := strings.TrimPrefix(r.URL.Path, "/"+provider)
			upstream := origin + path
			if len(query) > 0 {
				upstream += "?" + query.Encode()
			}
			req, err := http.NewRequestWithContext(upstreamCtx, "POST", upstream, strings.NewReader(string(body)))
			if err != nil {
				Error(w, 503, "UPSTREAM_UNAVAILABLE")
				return
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", r.Header.Get("Accept"))
			req.Header.Set("Authorization", "Bearer "+cred.AccessToken)
			if provider == "codex" {
				if cred.AccountID == "" || strings.ContainsAny(cred.AccountID, "\r\n") {
					Error(w, 503, "PROVIDER_LOGIN_REQUIRED")
					return
				}
				req.Header.Set("ChatGPT-Account-Id", cred.AccountID)
				for _, key := range []string{"Originator", "Version", "Session_id", "OpenAI-Beta", "X-Codex-Turn-Metadata"} {
					if v := r.Header.Get(key); v != "" {
						req.Header.Set(key, v)
					}
				}
			} else {
				for _, key := range []string{"Anthropic-Version", "Anthropic-Beta", "User-Agent", "X-App"} {
					if v := r.Header.Get(key); v != "" {
						req.Header.Set(key, v)
					}
				}
			}
			res, err := client.Do(req)
			if err != nil {
				Error(w, 503, "UPSTREAM_UNAVAILABLE")
				return
			}
			defer res.Body.Close()
			if res.StatusCode != 200 {
				if res.StatusCode == 401 || res.StatusCode == 403 {
					Error(w, 503, "PROVIDER_LOGIN_REQUIRED")
				} else {
					Error(w, 503, "UPSTREAM_REJECTED")
				}
				return
			}
			// Provider errors are never reflected. Successful inference is workspace
			// content, streamed directly without platform persistence or logging.
			w.Header().Set("Content-Type", res.Header.Get("Content-Type"))
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(200)
			buf := make([]byte, 32<<10)
			reader := io.LimitReader(res.Body, 32<<20)
			for {
				n, e := reader.Read(buf)
				if n > 0 {
					if _, err = w.Write(buf[:n]); err != nil {
						return
					}
					if f, ok := w.(http.Flusher); ok {
						f.Flush()
					}
				}
				if e != nil {
					return
				}
			}
		}
		if provider == "codex" {
			mux.HandleFunc("POST /codex/responses", handler)
			mux.HandleFunc("POST /codex/responses/compact", handler)
		} else {
			mux.HandleFunc("POST /claude/v1/messages", handler)
			mux.HandleFunc("POST /claude/v1/messages/count_tokens", handler)
		}
	}
	return nil
}

func safeSubscriptionPart(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}
