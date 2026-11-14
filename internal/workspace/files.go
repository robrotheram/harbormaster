package workspace

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"syscall"
	"time"

	"agent-workbench/internal/shared"
)

func (m *Manager) handoff(w http.ResponseWriter, r *http.Request) {
	var file shared.FileHandoff
	if !shared.Decode(w, r, &file, 1500<<10) {
		return
	}
	if !shared.ValidProjectFile(file.Path) || len(file.Content) > shared.HandoffLimit {
		shared.Error(w, 400, "INVALID_PROJECT_FILE")
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	a := m.workActivityLocked()
	if m.closed || m.quiescing || !a.Known || a.ActiveRuns > 0 || a.BackgroundJobs > 0 {
		shared.Error(w, 409, "WORKSPACE_BUSY")
		return
	}
	if r.URL.Path == "/v1/files/read" {
		f, err := m.root.OpenFile(file.Path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
		if err != nil {
			shared.Error(w, 404, "FILE_UNAVAILABLE")
			return
		}
		defer f.Close()
		if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() || info.Size() > shared.HandoffLimit {
			shared.Error(w, 400, "FILE_UNAVAILABLE")
			return
		}
		content, err := io.ReadAll(io.LimitReader(f, shared.HandoffLimit+1))
		if err != nil || len(content) > shared.HandoffLimit {
			shared.Error(w, 400, "FILE_UNAVAILABLE")
			return
		}
		m.lastActivity = time.Now().UTC()
		shared.JSON(w, 200, shared.FileHandoff{Path: file.Path, Content: content})
		return
	}
	if err := m.root.MkdirAll(path.Dir(file.Path), 0700); err != nil {
		shared.Error(w, 400, "INVALID_PROJECT_FILE")
		return
	}
	tmp := path.Join(path.Dir(file.Path), "."+shared.NewID("handoff"))
	f, err := m.root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		shared.Error(w, 503, "WRITE_FAILED")
		return
	}
	defer m.root.Remove(tmp)
	_, err = f.Write(file.Content)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		shared.Error(w, 503, "WRITE_FAILED")
		return
	}
	// Publish the complete file atomically without replacing any destination.
	if err = m.root.Link(tmp, file.Path); err != nil {
		if errors.Is(err, os.ErrExist) {
			shared.Error(w, 409, "DESTINATION_EXISTS")
		} else {
			shared.Error(w, 400, "INVALID_PROJECT_FILE")
		}
		return
	}
	m.lastActivity = time.Now().UTC()
	shared.JSON(w, 201, map[string]any{"copied": true, "bytes": len(file.Content)})
}
