package workspacesvc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"agent-workbench/internal/lifecycle"
	"agent-workbench/internal/runtime/protocol"
	"agent-workbench/internal/shared"
)

type StartCheck struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Status  string `json:"status"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}
type StartReadiness struct {
	WorkspaceID string       `json:"workspace_id"`
	ProjectID   string       `json:"project_id"`
	CheckedAt   time.Time    `json:"checked_at"`
	CanStart    bool         `json:"can_start"`
	Checks      []StartCheck `json:"checks"`
}

func checkNetwork(w shared.Workspace, c protocol.RuntimeCapability) error {
	for _, mode := range c.NetworkModes {
		if mode == w.EffectiveConnections().Mode {
			return nil
		}
	}
	return shared.CodedError{Status: shared.StatusServiceUnavailable, Code: "NETWORK_POLICY_UNAVAILABLE"}
}

// StartReadiness observes a copy. It never reserves capacity, writes metadata,
// reconciles connections, starts compute or contacts a model provider. Start
// still rechecks admission under the quota lock; this result is not a lease.
func (s *Service) StartReadiness(ctx context.Context, w shared.Workspace) StartReadiness {
	out := StartReadiness{WorkspaceID: w.ID, ProjectID: w.ProjectID, CheckedAt: s.Now(), CanStart: true, Checks: []StartCheck{}}
	add := func(id, label, status, code, message string) {
		out.Checks = append(out.Checks, StartCheck{id, label, status, code, message})
		if status != "passed" {
			out.CanStart = false
		}
	}
	if err := s.Refresh(ctx, &w); err != nil {
		add("state", "Worker state", "unknown", "RUNTIME_UNAVAILABLE", "Could not reach the runtime. Check Platform services, then check again.")
	} else if w.State != "STOPPED" {
		if w.Ready {
			add("state", "Worker state", "blocked", "ALREADY_RUNNING", "This worker is already running. Close this check and open the worker.")
		} else {
			add("state", "Worker state", "blocked", "NOT_READY", "The worker is transitioning or its state is unknown. Wait for it to settle, then check again.")
		}
	} else {
		add("state", "Worker state", "passed", "", "Stopped and available to start.")
	}
	profileOK := true
	if err := s.ResolveConnectionProfile(ctx, &w); err != nil {
		profileOK = false
		add("profile", "Connection profile", "blocked", "INVALID_CONNECTION_PROFILE", "The connection profile is unavailable. Choose an existing profile in worker settings.")
	}
	if w.AgentKind == "hermes" {
		_, err := s.Store.HermesSetup(ctx, w.ProjectID)
		if err != nil || s.HermesAvailable == nil || !s.HermesAvailable(ctx) {
			add("hermes", "Hermes setup", "blocked", "HERMES_SETUP_REQUIRED", "Save a model in Hermes setup and check that the Hermes runtime is installed.")
		} else {
			add("hermes", "Hermes setup", "passed", "", "Model configuration is saved and the Hermes engine is installed.")
		}
	}
	caps, ok := s.Runtime.(CapabilityRuntime)
	var c protocol.RuntimeCapability
	var err error
	if ok {
		c, err = caps.Capabilities(ctx)
	}
	if errors.Is(err, protocol.ErrCapabilitiesIncompatible) || (ok && err == nil && c.SchemaVersion != 1) {
		add("runtime", "Runtime compatibility", "blocked", "RUNTIME_CAPABILITIES_INCOMPATIBLE", "The runtime controller uses an incompatible capability format. Update the host controller to match the control plane, then check again.")
	} else if !ok || err != nil {
		add("runtime", "Runtime compatibility", "unknown", "RUNTIME_UNAVAILABLE", "Could not verify runtime capabilities. Check Platform services, then check again.")
	} else {
		add("runtime", "Runtime compatibility", "passed", "", "The controller reports a supported capability format.")
		requested := protocol.ResourceLimits{CPUCores: w.CPULimit, MemoryBytes: w.MemoryLimitBytes}
		if err := protocol.CheckResources(&requested, c.Resources); err != nil {
			add("resources", "Worker allocation", "blocked", "RESOURCE_LIMIT_UNAVAILABLE", fmt.Sprintf("This worker requests %g CPU and %d MiB. The installed template supports %g CPU and %d MiB. Use a matching worker template or update the runtime configuration.", w.CPULimit, w.MemoryLimitBytes/(1<<20), c.Resources.CPUCores, c.Resources.MemoryBytes/(1<<20)))
		} else {
			add("resources", "Worker allocation", "passed", "", "The runtime supports this worker's CPU and memory allocation.")
		}
		if profileOK {
			if lifecycle.CheckConnections(w) != nil || checkNetwork(w, c) != nil {
				add("network", "Connections", "blocked", "NETWORK_POLICY_UNAVAILABLE", "The runtime cannot apply these connections. Choose a supported profile in worker settings.")
			} else {
				add("network", "Connections", "passed", "", "The runtime supports the selected connection policy.")
			}
		}
		if w.ParentID != "" && !c.BranchSeed {
			add("branch", "Branch workspace", "blocked", "BRANCH_SEED_UNAVAILABLE", "This runtime cannot create a worker from another workspace. Create an independent worker instead.")
		}
	}
	if err := s.checkQuota(ctx, &w); err != nil {
		if errors.Is(err, shared.ErrQuotaExceeded) {
			add("capacity", "Factory capacity", "blocked", "QUOTA_EXCEEDED", "The worker or resource quota is full. Stop an idle worker in Infrastructure, then check again.")
		} else {
			add("capacity", "Factory capacity", "unknown", "STORAGE_UNAVAILABLE", "Could not read current allocations. Check Platform services, then check again.")
		}
	} else {
		add("capacity", "Factory capacity", "passed", "", "Current allocations leave enough capacity for this worker.")
	}
	return out
}
