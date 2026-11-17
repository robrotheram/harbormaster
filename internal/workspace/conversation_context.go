package workspace

import (
	"agent-workbench/internal/shared"
	"crypto/sha256"
	"fmt"
	"net/http"
	"os"
	"time"
)

// Capture the prior turn in the private workspace. Its instruction references
// older context files, so the chain survives model/provider changes without
// sending conversation bodies through the control plane or metadata store.
func (m *Manager) conversationContext(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !shared.ValidID(id) {
		shared.Error(w, 400, "INVALID_REQUEST")
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.quiescing {
		shared.Error(w, 503, "WORKSPACE_STOPPING")
		return
	}
	var instruction, result, state string
	if err := m.db.QueryRowContext(r.Context(), `SELECT instruction,result,state FROM tasks WHERE id=?`, id).Scan(&instruction, &result, &state); err != nil {
		shared.Error(w, 404, "TASK_NOT_FOUND")
		return
	}
	if state == "running" || state == "queued" || state == "cancel_requested" {
		shared.Error(w, 409, "WORKSPACE_BUSY")
		return
	}
	content := []byte("# Previous request and conversation context\n\n" + instruction + "\n\n# Agent reply\n\n" + result)
	if len(content) > 2<<20 {
		shared.Error(w, 400, "CONTEXT_TOO_LARGE")
		return
	}
	digest := sha256.Sum256(content)
	path := fmt.Sprintf("work-requests/context-%s-%x.md", id, digest[:8])
	if err := m.root.MkdirAll("work-requests", 0700); err != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	// Publish atomically. The digest names the immutable contents, including
	// interrupted runs whose partial results differ from later completions.
	tmp := "work-requests/." + shared.NewID("context")
	f, err := m.root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	defer m.root.Remove(tmp)
	_, err = f.Write(content)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	if err = m.root.Rename(tmp, path); err != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	m.lastActivity = time.Now().UTC()
	shared.JSON(w, 200, map[string]string{"task_id": id, "path": path})
}
