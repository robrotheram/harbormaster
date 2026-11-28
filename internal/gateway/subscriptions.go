package gateway

// Native CLI requests retain their provider protocol. Subscription credentials
// never cross this boundary into a workspace; its bearer is a workspace grant.
import (
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

type SubscriptionProvider struct {
	Models         []string      `json:"models"`
	DiscoverModels bool          `json:"discover_models,omitempty"`
	Network        NetworkPolicy `json:"network"`
}
type SubscriptionConfig struct {
	// Retain the field for existing saved-account configurations.
	UserAccounts bool                            `json:"user_accounts,omitempty"`
	Providers    map[string]SubscriptionProvider `json:"providers"`
}
type subscriptionCredential struct {
	AccessToken string `json:"access_token"`
	AccountID   string `json:"account_id,omitempty"`
	ExpiresAt   int64  `json:"expires_at"`
}

func (c subscriptionCredential) valid() bool {
	return c.AccessToken != "" && !strings.ContainsAny(c.AccessToken, " \r\n\t") && c.ExpiresAt > time.Now().Add(30*time.Second).Unix()
}

func registerSubscriptions(mux *http.ServeMux, c Config, catalog *codexCatalog) error {
	if c.Subscriptions == nil {
		return nil
	}
	if len(c.Subscriptions.Providers) == 0 || c.ResolveSubscription == nil {
		return ErrDenied
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
			grant := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			id, err := Authenticate(r, c.Verify, "inference")
			if err != nil {
				shared.Error(w, 401, "UNAUTHORIZED")
				return
			}
			query := r.URL.Query()
			for k, v := range query {
				if provider != "claude" || k != "beta" || len(v) != 1 || v[0] != "true" {
					shared.Error(w, 400, "INVALID_REQUEST")
					return
				}
			}
			body, err := ReadBounded(r.Body, 4<<20)
			fields, parseErr := shared.PolicyJSON(body)
			model, ok := fields["model"].(string)
			if err != nil || parseErr != nil || !ok || model == "" || (!models[model] && (provider != "codex" || catalog == nil)) {
				shared.Error(w, 400, "MODEL_NOT_ALLOWED")
				return
			}
			body, err = json.Marshal(fields)
			if err != nil {
				shared.Error(w, 400, "INVALID_REQUEST")
				return
			}
			// The registered route selects the upstream path. The gateway
			// supplies the provider identity and credential.
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
			defer cancel()
			cred, err := c.ResolveSubscription(ctx, id.WorkspaceID, provider, grant)
			if err != nil || !cred.valid() {
				shared.Error(w, 503, "PROVIDER_LOGIN_REQUIRED")
				return
			}
			if !models[model] {
				if _, available := catalog.models(ctx, cred)[model]; !available {
					shared.Error(w, 400, "MODEL_NOT_ALLOWED")
					return
				}
			}
			upstreamCtx, stop := context.WithDeadline(ctx, time.Unix(cred.ExpiresAt, 0))
			defer stop()
			// Native generations may exceed the server's short default write
			// timeout. Bound this response by its request/credential lifetime,
			// otherwise a healthy long generation is truncated during streaming.
			deadline, _ := upstreamCtx.Deadline()
			if err := http.NewResponseController(w).SetWriteDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
				shared.Error(w, 503, "UPSTREAM_UNAVAILABLE")
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
						current, err := c.ResolveSubscription(upstreamCtx, id.WorkspaceID, provider, grant)
						if err != nil || current.AccessToken != cred.AccessToken || current.AccountID != cred.AccountID {
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
				shared.Error(w, 503, "UPSTREAM_UNAVAILABLE")
				return
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", r.Header.Get("Accept"))
			req.Header.Set("Authorization", "Bearer "+cred.AccessToken)
			if provider == "codex" {
				if cred.AccountID == "" || strings.ContainsAny(cred.AccountID, "\r\n") {
					shared.Error(w, 503, "PROVIDER_LOGIN_REQUIRED")
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
				shared.Error(w, 503, "UPSTREAM_UNAVAILABLE")
				return
			}
			defer res.Body.Close()
			if res.StatusCode != 200 {
				if res.StatusCode == 401 || res.StatusCode == 403 {
					shared.Error(w, 503, "PROVIDER_LOGIN_REQUIRED")
				} else {
					shared.Error(w, 503, "UPSTREAM_REJECTED")
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
