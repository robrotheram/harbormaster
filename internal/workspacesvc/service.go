// Package workspacesvc coordinates workspace use cases. Callers serialize multi-step
// operations with Coordinator.Lock; quota reservations use a separate short lock.
package workspacesvc

import (
	"agent-workbench/internal/lifecycle"
	"agent-workbench/internal/runtime/protocol"
	"agent-workbench/internal/shared"
	"context"
	"errors"
	"log/slog"
	"sort"
	"sync"
	"time"
)

type Repository interface {
	Workspaces(context.Context) ([]shared.Workspace, error)
	Save(context.Context, shared.Workspace) error
	ConnectionProfile(context.Context, string) (shared.ConnectionProfile, error)
	HermesSetup(context.Context, string) (shared.HermesSetup, error)
}
type Runtime interface {
	Start(context.Context, string) (shared.Observation, error)
	Stop(context.Context, string) error
	Inspect(context.Context, string) (shared.Observation, error)
	Agent(context.Context, string, string, string, []byte) (int, []byte, error)
}
type Reconciler interface {
	Reconcile(context.Context, string) error
}
type ConfiguredRuntime interface {
	StartConfigured(context.Context, string, protocol.WorkspaceSpec) (shared.Observation, error)
}
type CapabilityRuntime interface {
	Capabilities(context.Context) (protocol.RuntimeCapability, error)
}

type ConnectionRuntime interface {
	ConfigureConnections(context.Context, string, shared.ConnectionPolicy) error
}
type LegacyNetworkRuntime interface {
	ConfigureNetwork(context.Context, string, bool) error
}
type Config struct {
	IdleTimeout time.Duration
	MaxRunning  int
	CPUQuota    float64
	MemoryQuota int64
}
type Service struct {
	Store           Repository
	Runtime         Runtime
	Config          Config
	Now             func() time.Time
	HermesAvailable func(context.Context) bool
	Coordinator     *Coordinator
}
type lockEntry struct {
	mu         sync.Mutex
	references int
}
type Coordinator struct {
	quotaMu sync.Mutex
	locksMu sync.Mutex
	locks   map[string]*lockEntry
}

// Lock acquires workspace locks in stable order, including both ends of handoffs.
func (c *Coordinator) Lock(ids ...string) func() {
	ids = append([]string(nil), ids...)
	sort.Strings(ids)
	locks := []*lockEntry{}
	keys := []string{}
	c.locksMu.Lock()
	if c.locks == nil {
		c.locks = map[string]*lockEntry{}
	}
	for i, id := range ids {
		if i > 0 && id == ids[i-1] {
			continue
		}
		m := c.locks[id]
		if m == nil {
			m = &lockEntry{}
			c.locks[id] = m
		}
		m.references++
		keys = append(keys, id)
		locks = append(locks, m)
	}
	c.locksMu.Unlock()
	for _, m := range locks {
		m.mu.Lock()
	}
	return func() {
		for i := len(locks) - 1; i >= 0; i-- {
			locks[i].mu.Unlock()
		}
		c.locksMu.Lock()
		defer c.locksMu.Unlock()
		for i, m := range locks {
			m.references--
			if m.references == 0 {
				delete(c.locks, keys[i])
			}
		}
	}
}
func Reserved(w shared.Workspace) bool { return w.State != "STOPPED" }
func (s *Service) Refresh(ctx context.Context, w *shared.Workspace) error {
	o, err := s.Runtime.Inspect(ctx, w.ID)
	if err != nil {
		w.State = "UNKNOWN"
		w.Ready = false
		w.IdleStopAt = nil
		return err
	}
	lifecycle.ApplyObservation(w, o, s.Now(), s.Config.IdleTimeout)
	return nil
}

// ObserveTask preserves the existing cold-read wake behavior, but polling an
// available task never renews the explicit user-open timestamp.
func (s *Service) ObserveTask(ctx context.Context, w *shared.Workspace) error {
	if err := s.Refresh(ctx, w); err != nil {
		return err
	}
	if !w.Ready {
		return s.start(ctx, w, false)
	}
	return nil
}
func (s *Service) report(stage string, w shared.Workspace, err error) {
	if err != nil {
		slog.Error("workspace persistence failed", "stage", stage, "workspace_id", w.ID, "code", "STORAGE_UNAVAILABLE")
	}
}
func (s *Service) Start(ctx context.Context, w *shared.Workspace) error { return s.start(ctx, w, true) }
func (s *Service) start(ctx context.Context, w *shared.Workspace, opened bool) error {
	var hermesModel string
	var hermesMatrix *shared.HermesMatrix
	if w.AgentKind == "hermes" {
		c, err := s.Store.HermesSetup(ctx, w.ProjectID)
		if err != nil || !s.HermesAvailable(ctx) {
			return errors.New("HERMES_SETUP_REQUIRED")
		}
		hermesModel = c.Model
		hermesMatrix = c.Matrix
	}
	if w.ConnectionProfileID != "" {
		if err := s.Refresh(ctx, w); err != nil {
			return err
		}
		if w.State == "STOPPED" {
			if err := s.ResolveConnectionProfile(ctx, w); err != nil {
				return err
			}
		}
	}
	if opened {
		now := s.Now()
		w.LastOpenedAt = &now
	}
	if e := s.Refresh(ctx, w); e != nil {
		return e
	}
	if err := lifecycle.CheckConnections(*w); err != nil {
		return err
	}
	if w.Ready {
		if reconciler, ok := s.Runtime.(Reconciler); ok {
			if err := reconciler.Reconcile(ctx, w.ID); err != nil {
				return err
			}
		}
		if err := s.ConfigureNetwork(ctx, *w); err != nil {
			return err
		}
		if err := s.RestoreConsole(ctx, *w); err != nil {
			return err
		}
		return s.Store.Save(ctx, *w)
	}
	if caps, ok := s.Runtime.(CapabilityRuntime); ok {
		c, err := caps.Capabilities(ctx)
		if err != nil {
			return err
		}
		if c.SchemaVersion > 0 {
			requested := protocol.ResourceLimits{CPUCores: w.CPULimit, MemoryBytes: w.MemoryLimitBytes}
			if err := protocol.CheckResources(&requested, c.Resources); err != nil {
				return err
			}
			supported := false
			for _, mode := range c.NetworkModes {
				if mode == w.EffectiveConnections().Mode {
					supported = true
					break
				}
			}
			if !supported {
				return errors.New("NETWORK_POLICY_UNAVAILABLE")
			}
		}
	}
	s.Coordinator.quotaMu.Lock()
	e := s.reserve(ctx, w)
	s.Coordinator.quotaMu.Unlock()
	if e != nil {
		return e
	}
	return s.startReserved(ctx, w, hermesModel, hermesMatrix)
}
func (s *Service) reserve(ctx context.Context, w *shared.Workspace) error {
	all, e := s.Store.Workspaces(ctx)
	if e != nil {
		return e
	}
	count := 0
	cpu := 0.0
	var mem int64
	for _, other := range all {
		if other.ID != w.ID && Reserved(other) {
			count++
			cpu += other.CPULimit
			mem += other.MemoryLimitBytes
		}
	}
	if count >= s.Config.MaxRunning || cpu+w.CPULimit > s.Config.CPUQuota || mem+w.MemoryLimitBytes > s.Config.MemoryQuota {
		return errors.New("QUOTA_EXCEEDED")
	}
	w.State = "STARTING"
	w.UpdatedAt = s.Now()
	if e = s.Store.Save(ctx, *w); e != nil {
		return e
	}
	return nil
}
func (s *Service) startReserved(ctx context.Context, w *shared.Workspace, hermesModel string, hermesMatrix *shared.HermesMatrix) error {
	var e error
	if configured, ok := s.Runtime.(ConfiguredRuntime); ok {
		_, e = configured.StartConfigured(ctx, w.ID, protocol.WorkspaceSpec{Resources: &protocol.ResourceLimits{CPUCores: w.CPULimit, MemoryBytes: w.MemoryLimitBytes}, HermesMatrix: hermesMatrix, HermesModel: hermesModel, ConnectionMode: w.EffectiveConnections().Mode, ProjectID: w.ProjectID, HomeID: w.HomeID, AgentKind: w.AgentKind, TemplateID: w.TemplateID, ConsoleEnabled: w.ConsoleEnabled, Name: w.Name, ParentID: w.ParentID, Branch: w.Branch})
	} else {
		_, e = s.Runtime.Start(ctx, w.ID)
	}
	if e != nil {
		w.State = "ERROR"
		w.Ready = false
		s.report("lifecycle", *w, s.Store.Save(ctx, *w))
		return e
	}
	if e = s.ConfigureNetwork(ctx, *w); e != nil {
		return e
	}
	if e = s.Refresh(ctx, w); e != nil {
		return e
	}
	if e = s.RestoreConsole(ctx, *w); e != nil {
		return e
	}
	return s.Store.Save(ctx, *w)
}
func (s *Service) Stop(ctx context.Context, w *shared.Workspace) error {
	if e := s.Refresh(ctx, w); e != nil {
		return e
	}
	if w.State == "STOPPED" {
		return s.Store.Save(ctx, *w)
	}
	if w.State != "READY" || w.ActiveRuns > 0 || w.BackgroundJobs > 0 {
		return errors.New("WORKSPACE_BUSY")
	}
	w.State = "STOPPING"
	w.Ready = false
	if e := s.Store.Save(ctx, *w); e != nil {
		return e
	}
	if e := s.Runtime.Stop(ctx, w.ID); e != nil {
		w.State = "UNKNOWN"
		s.report("lifecycle", *w, s.Store.Save(ctx, *w))
		return e
	}
	w.State = "STOPPED"
	w.IdleStopAt = nil
	w.UpdatedAt = s.Now()
	return s.Store.Save(ctx, *w)
}
func (s *Service) ResolveConnectionProfile(ctx context.Context, w *shared.Workspace) error {
	if w.ConnectionProfileID == "" {
		return nil
	}
	if !shared.ValidID(w.ConnectionProfileID) {
		return errors.New("INVALID_CONNECTION_PROFILE")
	}
	p, err := s.Store.ConnectionProfile(ctx, w.ConnectionProfileID)
	if err != nil {
		return errors.New("INVALID_CONNECTION_PROFILE")
	}
	w.Connections = &p.Policy
	w.ConnectionProfileRevision = p.Revision
	w.GitHubAccess = p.Policy.Allows("github.com:443")
	return nil
}
func (s *Service) ConfigureNetwork(ctx context.Context, w shared.Workspace) error {
	if rt, ok := s.Runtime.(ConnectionRuntime); ok {
		return rt.ConfigureConnections(ctx, w.ID, w.EffectiveConnections())
	}
	if w.Connections != nil && (w.Connections.Mode == "open" || len(w.Connections.Destinations) > 0 && (len(w.Connections.Destinations) != 1 || !w.Connections.Allows("github.com:443"))) {
		return errors.New("NETWORK_POLICY_UNAVAILABLE")
	}

	if rt, ok := s.Runtime.(LegacyNetworkRuntime); ok {
		return rt.ConfigureNetwork(ctx, w.ID, w.GitHubAccess)
	}
	if w.GitHubAccess {
		return errors.New("NETWORK_POLICY_UNAVAILABLE")
	}
	return nil
}
