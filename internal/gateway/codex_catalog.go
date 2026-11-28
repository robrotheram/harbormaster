package gateway

import (
	"agent-workbench/internal/shared"
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

type catalogEntry struct {
	models  map[string][]string
	expires time.Time
}

type codexCatalog struct {
	mu     sync.Mutex
	cache  map[[32]byte]catalogEntry
	client *http.Client
}

func (c *codexCatalog) models(ctx context.Context, cred subscriptionCredential) map[string][]string {
	if c == nil || cred.AccountID == "" || cred.AccessToken == "" {
		return nil
	}
	key := sha256.Sum256([]byte(cred.AccountID + "\x00" + cred.AccessToken))
	c.mu.Lock()
	entry := c.cache[key]
	c.mu.Unlock()
	if time.Now().Before(entry.expires) {
		return entry.models
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	// Keep this in sync with the native Codex client pinned in the workspace image.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://chatgpt.com/backend-api/codex/models?client_version=0.164.0", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+cred.AccessToken)
	req.Header.Set("ChatGPT-Account-Id", cred.AccountID)
	res, err := c.client.Do(req)
	if err != nil {
		return nil
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil
	}
	raw, err := ReadBounded(res.Body, 1<<20)
	if err != nil {
		return nil
	}
	var result struct {
		Models []struct {
			Slug       string `json:"slug"`
			Visibility string `json:"visibility"`
			Levels     []struct {
				Effort string `json:"effort"`
			} `json:"supported_reasoning_levels"`
		} `json:"models"`
	}
	if json.Unmarshal(raw, &result) != nil {
		return nil
	}
	models := make(map[string][]string)
	for _, m := range result.Models {
		if m.Visibility != "list" || m.Slug == "" || !shared.ValidWorkSelection("codex", m.Slug) {
			continue
		}
		levels := []string{}
		for _, level := range m.Levels {
			if level.Effort != "" && shared.ValidReasoningEffort("codex", level.Effort) {
				levels = append(levels, level.Effort)
			}
		}
		models[m.Slug] = levels
	}
	c.mu.Lock()
	if len(c.cache) >= 256 {
		clear(c.cache)
	}
	c.cache[key] = catalogEntry{models: models, expires: time.Now().Add(2 * time.Minute)}
	c.mu.Unlock()
	return models
}

func newCodexCatalog(c Config) (*codexCatalog, error) {
	if c.Subscriptions == nil {
		return nil, nil
	}
	policy := c.Subscriptions.Providers["codex"]
	if !policy.DiscoverModels {
		return nil, nil
	}
	tr, err := policy.Network.Transport(nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Transport: tr, Timeout: 3 * time.Second, CheckRedirect: NoRedirect}
	if c.subscriptionHTTP != nil {
		client = c.subscriptionHTTP
	}
	return &codexCatalog{cache: make(map[[32]byte]catalogEntry), client: client}, nil
}
