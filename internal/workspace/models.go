package workspace

import (
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

func (m *Manager) agentModels(ctx context.Context) (shared.AgentModels, error) {
	out := shared.AgentModels{WorkspaceID: m.cfg.ID, DefaultProvider: m.cfg.DefaultProvider, Items: []shared.AgentModel{}}
	if out.DefaultProvider == "codex" {
		out.DefaultModel = m.cfg.CodexModel
	}
	if out.DefaultProvider == "claude" {
		out.DefaultModel = m.cfg.ClaudeModel
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(m.cfg.InferenceURL, "/")+"/v1/models", nil)
	if err != nil {
		return out, err
	}
	req.Header.Set("Authorization", "Bearer "+m.cfg.Token)
	client := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if m.cfg.HTTP != nil {
		client = m.cfg.HTTP
	}
	res, err := client.Do(req)
	if err != nil {
		return out, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return out, fmt.Errorf("model catalog unavailable")
	}
	var catalog shared.AgentModels
	if err = json.NewDecoder(io.LimitReader(res.Body, 256<<10)).Decode(&catalog); err != nil {
		return out, err
	}
	for _, selection := range catalog.Items {
		if !shared.ValidWorkSelection(selection.Provider, selection.Model) || selection.Model == "" {
			continue
		}
		if selection.Provider != "codex" && selection.Provider != "claude" {
			continue
		}
		if _, err := exec.LookPath(selection.Provider); err != nil {
			continue
		}
		out.Items = append(out.Items, selection)
	}
	return out, nil
}
func (m *Manager) models(w http.ResponseWriter, r *http.Request) {
	catalog, err := m.agentModels(r.Context())
	if err != nil {
		shared.Error(w, 503, "MODEL_CATALOG_UNAVAILABLE")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	shared.JSON(w, 200, catalog)
}
