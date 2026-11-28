package gateway

import (
	"agent-workbench/internal/shared"
	"context"
	"net/http"
	"sort"
	"strings"
	"time"
)

func registerModels(mux *http.ServeMux, c Config, catalog *codexCatalog) {
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		grant := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		id, err := Authenticate(r, c.Verify, "inference")
		if err != nil {
			shared.Error(w, 401, "UNAUTHORIZED")
			return
		}
		out := shared.AgentModels{Items: []shared.AgentModel{}, ReasoningEfforts: map[string][]string{}}
		if c.Subscriptions != nil {
			for provider, policy := range c.Subscriptions.Providers {
				ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
				cred, err := c.ResolveSubscription(ctx, id.WorkspaceID, provider, grant)
				cancel()
				if err != nil || !cred.valid() {
					continue
				}
				available := map[string]bool{}
				for _, model := range policy.Models {
					available[model] = true
				}
				if provider == "codex" {
					for model, efforts := range catalog.models(r.Context(), cred) {
						available[model] = true
						out.ReasoningEfforts[provider+"/"+model] = efforts
					}
				}
				for model := range available {
					if shared.ValidWorkSelection(provider, model) {
						out.Items = append(out.Items, shared.AgentModel{Provider: provider, Model: model})
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
