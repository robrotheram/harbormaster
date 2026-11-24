package workspace

import (
	"context"
	"net/http"
	"time"

	"agent-workbench/internal/shared"
)

// An analysis request prevents idle stop while it is using worker storage.
func (m *Manager) analysisActivity(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		if m.closed || m.quiescing {
			m.mu.Unlock()
			shared.Error(w, 409, "WORKSPACE_BUSY")
			return
		}
		m.analysisJobs++
		m.wg.Add(1)
		m.mu.Unlock()
		defer func() {
			m.mu.Lock()
			m.analysisJobs--
			m.lastActivity = time.Now().UTC()
			m.mu.Unlock()
			m.wg.Done()
		}()
		ctx, cancel := context.WithCancel(r.Context())
		done := make(chan struct{})
		stop := context.AfterFunc(m.analysisCtx, func() {
			defer close(done)
			cancel()
			_ = http.NewResponseController(w).SetReadDeadline(time.Now())
		})
		defer func() {
			if !stop() {
				<-done
			}
		}()
		defer cancel()
		next(w, r.WithContext(ctx))
	}
}
