package gateway

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"time"
)

type Config struct {
	ResolveSubscription    func(context.Context, string) (subscriptionCredential, error) `json:"-"`
	Mode                   string                                                        `json:"-"`
	Models                 []string                                                      `json:"models"`
	Origin                 string                                                        `json:"origin,omitempty"`
	Network                NetworkPolicy                                                 `json:"network,omitempty"`
	APIKey                 string                                                        `json:"-"`
	Verify                 Verifier                                                      `json:"-"`
	Subscriptions          *SubscriptionConfig                                           `json:"subscriptions,omitempty"`
	SubscriptionVaultAddr  string                                                        `json:"-"`
	SubscriptionVaultCA    string                                                        `json:"-"`
	SubscriptionVaultToken string                                                        `json:"-"`
	subscriptionHTTP       *http.Client
}

func ConfigFromEnv() (Config, error) {
	var c Config
	if err := ReadConfig(os.Getenv("GATEWAY_CONFIG"), &c); err != nil {
		return c, err
	}
	c.Mode = os.Getenv("GATEWAY_MODE")
	token, err := ReadSecret(os.Getenv("GATEWAY_SERVICE_TOKEN_FILE"))
	if err != nil {
		return c, err
	}
	controller := os.Getenv("CONTROLLER_URL")
	if controller == "" {
		controller = "http://runtime-controller:8080"
	}
	c.Verify, err = NewVerifier(controller, token)
	if err != nil {
		return c, err
	}
	if c.Subscriptions != nil && c.Subscriptions.UserAccounts {
		c.ResolveSubscription, err = newSubscriptionResolver(os.Getenv("CONTROLPLANE_URL"), token)
		if err != nil {
			return c, err
		}
	}
	if c.Subscriptions != nil && !c.Subscriptions.UserAccounts {
		c.SubscriptionVaultAddr = os.Getenv("SUBSCRIPTION_VAULT_ADDR")
		c.SubscriptionVaultCA = os.Getenv("SUBSCRIPTION_VAULT_CA_FILE")
		c.SubscriptionVaultToken, err = ReadSecret(os.Getenv("SUBSCRIPTION_VAULT_TOKEN_FILE"))
		if err != nil {
			return c, err
		}
	}
	if c.Mode == "upstream" {
		c.APIKey, err = ReadSecret(os.Getenv("INFERENCE_API_KEY_FILE"))
	}
	return c, err
}
func NewHandler(c Config) (http.Handler, error) {
	if (c.Mode == "subscription" && c.Subscriptions == nil) || c.Verify == nil || (c.Mode != "fixture" && c.Mode != "upstream" && c.Mode != "subscription") || len(c.Models) == 0 {
		return nil, ErrDenied
	}
	models := map[string]bool{}
	for _, m := range c.Models {
		if m == "" {
			return nil, ErrDenied
		}
		models[m] = true
	}
	var client *http.Client
	endpoint := ""
	if c.Mode == "upstream" {
		u, err := HTTPSOrigin(c.Origin)
		if err != nil || c.APIKey == "" {
			return nil, ErrDenied
		}
		u.Path = "/v1/chat/completions"
		endpoint = u.String()
		tr, err := c.Network.Transport(nil)
		if err != nil {
			return nil, err
		}
		client = &http.Client{Transport: tr, Timeout: 30 * time.Second, CheckRedirect: NoRedirect}
	}
	mux := http.NewServeMux()
	if err := registerSubscriptions(mux, c); err != nil {
		return nil, err
	}
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { JSON(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if c.Mode == "subscription" {
			Error(w, 503, "PROVIDER_NOT_CONFIGURED")
			return
		}
		if _, err := Authenticate(r, c.Verify, "inference"); err != nil {
			Error(w, 401, "UNAUTHORIZED")
			return
		}
		b, err := ReadBounded(r.Body, 1<<20)
		if err != nil {
			Error(w, 400, "INVALID_REQUEST")
			return
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(b, &fields) != nil || fields == nil {
			Error(w, 400, "INVALID_REQUEST")
			return
		}
		// Unknown top-level keys cannot become alternate routing or identity controls.
		allowed := map[string]bool{"model": true, "messages": true, "tools": true, "tool_choice": true, "temperature": true, "top_p": true, "max_tokens": true, "max_completion_tokens": true, "stop": true, "seed": true, "response_format": true, "frequency_penalty": true, "presence_penalty": true, "n": true, "parallel_tool_calls": true, "stream": true}
		for k := range fields {
			if !allowed[k] {
				Error(w, 400, "INVALID_REQUEST")
				return
			}
		}
		var model string
		var stream bool
		var messages []map[string]json.RawMessage
		if json.Unmarshal(fields["model"], &model) != nil || !models[model] {
			Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		if v, ok := fields["stream"]; ok {
			if json.Unmarshal(v, &stream) != nil {
				Error(w, 400, "INVALID_REQUEST")
				return
			}
		}
		if stream {
			Error(w, 400, "STREAMING_UNSUPPORTED")
			return
		}
		if json.Unmarshal(fields["messages"], &messages) != nil || len(messages) == 0 || len(messages) > 256 {
			Error(w, 400, "INVALID_REQUEST")
			return
		}
		for _, m := range messages {
			var role string
			if json.Unmarshal(m["role"], &role) != nil || (role != "system" && role != "developer" && role != "user" && role != "assistant" && role != "tool") {
				Error(w, 400, "INVALID_REQUEST")
				return
			}
		}
		if c.Mode == "fixture" {
			sum := sha256.Sum256(fields["messages"])
			JSON(w, 200, map[string]any{"id": "chatcmpl-fixture-" + hex.EncodeToString(sum[:8]), "object": "chat.completion", "created": 0, "model": "fixture", "choices": []any{map[string]any{"index": 0, "message": map[string]string{"role": "assistant", "content": "[SYNTHETIC FIXTURE] messages_sha256=" + hex.EncodeToString(sum[:])}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0}})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(b))
		if err != nil {
			Error(w, 503, "UPSTREAM_UNAVAILABLE")
			return
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Content-Type", "application/json")
		res, err := client.Do(req)
		if err != nil {
			Error(w, 503, "UPSTREAM_UNAVAILABLE")
			return
		}
		defer res.Body.Close()
		response, err := ReadBounded(res.Body, 4<<20)
		if err != nil || res.StatusCode != 200 || !json.Valid(response) {
			Error(w, 503, "UPSTREAM_UNAVAILABLE")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(200)
		_, _ = w.Write(response)
	})
	return mux, nil
}
