package workspace

import (
	"net/http"
	"os"
	"path/filepath"

	"agent-workbench/internal/shared"
)

func (m *Manager) enableConsole(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.console != nil {
		shared.JSON(w, 200, map[string]bool{"enabled": true})
		return
	}
	a := m.activityLocked()
	if m.closed || m.quiescing || !a.Known || a.ActiveRuns > 0 || a.BackgroundJobs > 0 {
		shared.Error(w, 409, "WORKSPACE_BUSY")
		return
	}
	if err := m.startConsole(); err != nil {
		shared.Error(w, 503, "CONSOLE_UNAVAILABLE")
		return
	}
	if err := os.WriteFile(filepath.Join(m.homePath(), ".t3-enabled"), []byte("enabled"), 0600); err != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	shared.JSON(w, 200, map[string]bool{"enabled": true})
}
