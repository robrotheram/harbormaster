package api

import (
	"agent-workbench/internal/shared"
	"time"
)

// snapshotWorkspace is an allowlist: adding a private configuration field to
// shared.Workspace cannot silently publish it on the unauthenticated listener.
type snapshotWorkspace struct {
	ID               string     `json:"id"`
	ProjectID        string     `json:"project_id"`
	State            string     `json:"state"`
	Ready            bool       `json:"ready"`
	ActiveRuns       int        `json:"active_runs"`
	BackgroundJobs   int        `json:"background_jobs"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	IdleStopAt       *time.Time `json:"idle_stop_at"`
	RuntimeType      string     `json:"runtime_type"`
	IsolationStatus  string     `json:"isolation_status"`
	NetworkStatus    string     `json:"network_status"`
	CPULimit         float64    `json:"cpu_limit"`
	MemoryLimitBytes int64      `json:"memory_limit_bytes"`
	CPUCores         *float64   `json:"cpu_cores"`
	MemoryBytes      *int64     `json:"memory_bytes"`
	ObservedAt       *time.Time `json:"observed_at"`
}

func snapshotWorkspaces(items []shared.Workspace) []snapshotWorkspace {
	out := make([]snapshotWorkspace, 0, len(items))
	for _, w := range items {
		out = append(out, snapshotWorkspace{ID: w.ID, ProjectID: w.ProjectID, State: w.State, Ready: w.Ready, ActiveRuns: w.ActiveRuns, BackgroundJobs: w.BackgroundJobs, CreatedAt: w.CreatedAt, UpdatedAt: w.UpdatedAt, IdleStopAt: w.IdleStopAt, RuntimeType: w.RuntimeType, IsolationStatus: w.IsolationStatus, NetworkStatus: w.NetworkStatus, CPULimit: w.CPULimit, MemoryLimitBytes: w.MemoryLimitBytes, CPUCores: w.CPUCores, MemoryBytes: w.MemoryBytes, ObservedAt: w.ObservedAt})
	}
	return out
}
