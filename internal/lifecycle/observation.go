// Package lifecycle defines workspace lifecycle policy without transport or storage dependencies.
package lifecycle

import (
	"agent-workbench/internal/shared"
	"time"
)

// ApplyObservation updates the operational projection. Unknown activity never enables idle stopping.
func ApplyObservation(w *shared.Workspace, o shared.Observation, now time.Time, idleTimeout time.Duration) {
	w.RuntimeType = o.RuntimeType
	w.IsolationStatus = o.IsolationStatus
	w.NetworkStatus = o.NetworkStatus
	w.GitProxyReady = o.GitProxyReady
	w.ConnectionMode = o.ConnectionMode
	w.ConnectionProxyVersion = o.ConnectionProxyVersion
	w.ObservedAt = &now
	w.CPUCores = o.CPUCores
	w.MemoryBytes = o.MemoryBytes
	w.Ready = o.Ready
	w.ActiveRuns = o.Activity.ActiveRuns
	w.BackgroundJobs = o.Activity.BackgroundJobs
	if !o.Running {
		w.State = "STOPPED"
		w.Ready = false
		w.IdleStopAt = nil
		w.ActiveRuns = 0
		w.BackgroundJobs = 0
	} else if !o.Ready {
		w.State = "STARTING"
		w.IdleStopAt = nil
	} else if !o.Activity.Known {
		w.State = "UNKNOWN"
		w.IdleStopAt = nil
	} else if w.ActiveRuns > 0 || w.BackgroundJobs > 0 {
		w.State = "BUSY"
		w.IdleStopAt = nil
	} else {
		if w.State == "BUSY" {
			w.LastOpenedAt = &now
		}
		w.State = "READY"
		last := o.Activity.LastActivity
		if w.LastOpenedAt != nil && w.LastOpenedAt.After(last) {
			last = *w.LastOpenedAt
		}
		if last.IsZero() {
			w.State = "UNKNOWN"
			w.IdleStopAt = nil
		} else {
			timeout := idleTimeout
			if w.IdleTimeoutMinutes != nil {
				timeout = time.Duration(*w.IdleTimeoutMinutes) * time.Minute
			}
			w.IdleStopAt = nil
			if timeout > 0 {
				deadline := last.Add(timeout)
				w.IdleStopAt = &deadline
			}
		}
	}
}
