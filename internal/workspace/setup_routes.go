package workspace

import (
	"context"
	"errors"
	"net/http"
	"time"

	"agent-workbench/internal/shared"
	"agent-workbench/internal/workspace/setup"
)

type setupJob struct {
	ID        string           `json:"id"`
	State     string           `json:"state"`
	Code      string           `json:"code,omitempty"`
	Review    *setup.Review    `json:"review,omitempty"`
	Discovery *setup.Discovery `json:"discovery,omitempty"`
	Request   *setup.Request   `json:"request,omitempty"`
	plan      *setup.Plan
}

func (m *Manager) workerSetup() setup.Service {
	return setup.Service{
		Data: m.cfg.DataDir, Home: m.homePath(), Proxy: m.cfg.EgressURL != "" && m.cfg.ConnectionMode != "open",
		Blocked: m.cfg.ConnectionMode == "blocked",
		Skills:  m.cfg.AgentKind != "hermes",
		Mise:    "/usr/local/bin/mise", CLI: "/usr/local/bin/skills",
	}
}

func (m *Manager) setupRoute(w http.ResponseWriter, r *http.Request) {
	service := m.workerSetup()
	if !service.Available() {
		writeSetupError(w, setup.ErrUnavailable)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodGet {
		m.mu.Lock()
		defer m.mu.Unlock()
		inventory, err := service.Inventory(r.Context())
		if err != nil {
			writeSetupError(w, err)
			return
		}
		shared.JSON(w, 200, struct {
			setup.Inventory
			Job *setupJob `json:"job,omitempty"`
		}{inventory, m.setupJob})
		return
	}
	var req setup.Request
	if !shared.Decode(w, r, &req, 4096) {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.quiescing {
		shared.Error(w, 409, "WORKSPACE_STOPPING")
		return
	}
	if m.setupCancel != nil {
		shared.Error(w, 409, "SETUP_BUSY")
		return
	}
	if req.Operation == "apply" || req.Operation == "discard" {
		m.finishSetup(w, r, service, req)
		return
	}
	if err := service.Validate(&req); err != nil || req.PlanID != "" {
		writeSetupError(w, setup.ErrInvalid)
		return
	}
	if m.setupJob != nil && m.setupJob.State == "review" {
		shared.Error(w, 409, "SETUP_REVIEW_PENDING")
		return
	}
	if a := m.activityLocked(); !a.Known || a.ActiveRuns > 0 || a.BackgroundJobs > 0 {
		shared.Error(w, 409, "WORKSPACE_BUSY")
		return
	}
	m.setupJob = &setupJob{ID: shared.NewID("setup_"), State: "preparing", Request: &req}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	m.setupCancel = cancel
	m.lastActivity = time.Now().UTC()
	m.wg.Add(1)
	go m.prepareSetup(ctx, service, req, m.setupJob.ID)
	shared.JSON(w, 202, m.setupJob)
}

func (m *Manager) prepareSetup(ctx context.Context, service setup.Service, req setup.Request, id string) {
	defer m.wg.Done()
	job := &setupJob{ID: id, Request: &req}
	var err error
	if req.Operation == "skill-discover" {
		job.Discovery, err = service.Discover(ctx, req)
		job.State = "discovered"
	} else {
		job.plan, err = service.Prepare(ctx, id, req)
		job.State = "review"
		if err == nil {
			job.Review = &job.plan.Review
		}
	}
	if err != nil {
		job.State, job.Code = "failed", err.Error()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setupCancel()
	m.setupCancel = nil
	m.setupJob = job
	m.lastActivity = time.Now().UTC()
}

func (m *Manager) finishSetup(w http.ResponseWriter, r *http.Request, service setup.Service, req setup.Request) {
	job := m.setupJob
	if job == nil || job.ID != req.PlanID {
		writeSetupError(w, setup.ErrChanged)
		return
	}
	if req.Operation == "apply" && job.State == "applied" {
		shared.JSON(w, 200, job)
		return
	}
	if req.Operation == "discard" && job.State == "discarded" {
		shared.JSON(w, 200, job)
		return
	}
	if req.Operation == "discard" && job.State == "discovered" {
		job.State, job.Discovery = "discarded", nil
		shared.JSON(w, 200, job)
		return
	}
	if job.State != "review" {
		writeSetupError(w, setup.ErrChanged)
		return
	}
	if req.Operation == "apply" {
		if a := m.activityLocked(); !a.Known || a.ActiveRuns > 0 || a.BackgroundJobs > 0 {
			shared.Error(w, 409, "WORKSPACE_BUSY")
			return
		}
		// The publish phase is bounded local I/O. Let it complete even if the
		// browser closes while the confirmed request is in flight.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 15*time.Second)
		defer cancel()
		if err := service.Apply(ctx, job.plan); err != nil {
			job.State, job.Code = "failed", err.Error()
			job.plan.Close()
			job.plan, job.Review = nil, nil
			writeSetupError(w, err)
			return
		}
		job.State = "applied"
	} else {
		job.State = "discarded"
	}
	job.plan.Close()
	job.plan, job.Review = nil, nil
	m.lastActivity = time.Now().UTC()
	shared.JSON(w, 200, job)
}

func writeSetupError(w http.ResponseWriter, err error) {
	status := http.StatusConflict
	switch {
	case errors.Is(err, setup.ErrInvalid):
		status = http.StatusBadRequest
	case errors.Is(err, setup.ErrUnavailable), errors.Is(err, setup.ErrStorage):
		status = http.StatusServiceUnavailable
	}
	shared.Error(w, status, err.Error())
}
