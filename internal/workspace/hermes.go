package workspace

import (
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func (m *Manager) runtimeCapabilities(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	home := filepath.Join(m.homePath(), "agents", "hermes")
	_ = os.MkdirAll(home, 0700)
	c := exec.CommandContext(ctx, "/usr/local/bin/hermes-runner", "--version")
	c.Env = []string{"PATH=/opt/hermes/.venv/bin:/usr/local/bin:/usr/bin:/bin", "HOME=" + home, "HERMES_HOME=" + home, "PYTHONPATH=/opt/hermes"}
	out := &nativeOutput{}
	c.Stdout = out
	err := c.Run()
	var identity struct {
		Version string `json:"version"`
	}
	available := err == nil && !out.Overflow && json.Unmarshal(out.Data, &identity) == nil && identity.Version == "v0.23.0"
	reason := ""
	if ctx.Err() != nil {
		reason = "IMPORT_TIMEOUT"
	} else if !available {
		reason = "IMPORT_FAILED"
	}
	shared.JSON(w, 200, map[string]any{"hermes": available, "hermes_version": identity.Version, "error_code": reason})
}
