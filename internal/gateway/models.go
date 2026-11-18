package gateway

import (
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"
)

func registerModels(mux *http.ServeMux, c Config) {
	var vault *http.Client
	if c.Subscriptions != nil && !c.Subscriptions.UserAccounts {
		// Configuration validation in registerSubscriptions rejects invalid TLS.
		vault, _ = subscriptionVaultClient(c.SubscriptionVaultAddr, c.SubscriptionVaultCA)
	}
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		id, err := Authenticate(r, c.Verify, "inference")
		if err != nil {
			shared.Error(w, 401, "UNAUTHORIZED")
			return
		}
		out := shared.AgentModels{Items: []shared.AgentModel{}}
		if c.Subscriptions != nil && safeSubscriptionPart(id.PrincipalID) {
			for provider, policy := range c.Subscriptions.Providers {
				allowed := false
				if c.Subscriptions.UserAccounts {
					ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
					cred, err := c.ResolveSubscription(ctx, id.WorkspaceID, provider)
					cancel()
					allowed = err == nil && cred.AccessToken != "" && !strings.ContainsAny(cred.AccessToken, " \r\n\t") && cred.ExpiresAt > time.Now().Add(30*time.Second).Unix()
				} else {
					allowed = slices.Contains(c.Subscriptions.Principals[id.PrincipalID], provider)
				}
				if allowed && !c.Subscriptions.UserAccounts {
					allowed = catalogCredentialAvailable(r.Context(), vault, c, id.PrincipalID, provider)
				}
				if allowed {
					for _, model := range policy.Models {
						if shared.ValidWorkSelection(provider, model) {
							out.Items = append(out.Items, shared.AgentModel{Provider: provider, Model: model})
						}
					}
				}
			}
		}
		sort.Slice(out.Items, func(i, j int) bool {
			if out.Items[i].Provider == out.Items[j].Provider {
				return out.Items[i].Model < out.Items[j].Model
			}
			return out.Items[i].Provider < out.Items[j].Provider
		})
		w.Header().Set("Cache-Control", "private, no-store")
		shared.JSON(w, 200, out)
	})
}

func catalogCredentialAvailable(ctx context.Context, client *http.Client, c Config, principal, provider string) bool {
	if client == nil || !safeSubscriptionPart(principal) || !safeSubscriptionPart(provider) {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(c.SubscriptionVaultAddr, "/")+"/v1/subscription-auth/data/"+principal+"/"+provider, nil)
	if err != nil {
		return false
	}
	req.Header.Set("X-Vault-Token", c.SubscriptionVaultToken)
	res, err := client.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	raw, err := ReadBounded(res.Body, 64<<10)
	var secret struct {
		Data struct {
			Data subscriptionCredential `json:"data"`
		} `json:"data"`
	}
	if err != nil || res.StatusCode != 200 || json.Unmarshal(raw, &secret) != nil {
		return false
	}
	cred := secret.Data.Data
	return cred.AccessToken != "" && !strings.ContainsAny(cred.AccessToken, " \r\n\t") && cred.ExpiresAt > time.Now().Add(30*time.Second).Unix()
}
