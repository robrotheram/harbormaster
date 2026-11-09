package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/runtime/protocol"
	"agent-workbench/internal/shared"
	"agent-workbench/internal/store"
	"agent-workbench/internal/workspacesvc"
)

type Config struct {
	Token           string
	ProjectTokens   map[string]string
	IdleTimeout     time.Duration
	MaxRunning      int
	CPUQuota        float64
	MemoryQuota     int64
	WorkspaceMemory int64
	HealthEndpoints map[string]string
}
type Server struct {
	schedulerCursor orchestration.Cursor
	eventSeen       map[string]map[string]bool
	eventState      map[string]string
	matrixSetup     matrixOnboarding
	Store           *store.Store
	Runtime         protocol.Runtime
	cfg             Config
	mu              sync.Mutex
	createMu        sync.Mutex
	eventsMu        sync.Mutex
	operations      workspacesvc.Coordinator
	now             func() time.Time
	health          map[string]map[string]any
}

func New(s *store.Store, rt protocol.Runtime, c Config) *Server {
	if c.IdleTimeout == 0 {
		c.IdleTimeout = 60 * time.Minute
	}
	if c.MaxRunning == 0 {
		c.MaxRunning = 4
	}
	if c.CPUQuota == 0 {
		c.CPUQuota = 4
	}
	if c.MemoryQuota == 0 {
		c.MemoryQuota = 2 << 30
	}
	if c.WorkspaceMemory == 0 {
		c.WorkspaceMemory = 512 << 20
	}
	tokens := map[string]string{}
	for token, project := range c.ProjectTokens {
		tokens[token] = project
	}
	c.ProjectTokens = tokens
	return &Server{Store: s, Runtime: rt, cfg: c, now: func() time.Time { return time.Now().UTC() }, health: map[string]map[string]any{}}
}
func reserved(w shared.Workspace) bool { return workspacesvc.Reserved(w) }
func (s *Server) lifecycle() *workspacesvc.Service {
	return &workspacesvc.Service{Store: s.Store, Runtime: s.Runtime, Config: workspacesvc.Config{IdleTimeout: s.cfg.IdleTimeout, MaxRunning: s.cfg.MaxRunning, CPUQuota: s.cfg.CPUQuota, MemoryQuota: s.cfg.MemoryQuota}, Now: s.now, HermesAvailable: s.hermesAvailable, Coordinator: &s.operations}
}
func (s *Server) refresh(ctx context.Context, w *shared.Workspace) error {
	return s.lifecycle().Refresh(ctx, w)
}
func (s *Server) start(ctx context.Context, w *shared.Workspace) error {
	return s.lifecycle().Start(ctx, w)
}
func (s *Server) stop(ctx context.Context, w *shared.Workspace) error {
	return s.lifecycle().Stop(ctx, w)
}
func apiError(w http.ResponseWriter, e error) {
	switch e.Error() {
	case "INVALID_CONNECTION_PROFILE":
		shared.Error(w, 400, "INVALID_CONNECTION_PROFILE")
	case "QUOTA_EXCEEDED":
		shared.Error(w, 429, "QUOTA_EXCEEDED")
	case "ASSISTANT_WORKSPACE_AMBIGUOUS":
		shared.Error(w, 409, "ASSISTANT_WORKSPACE_AMBIGUOUS")
	case "WORKSPACE_BUSY":
		shared.Error(w, 409, "WORKSPACE_BUSY")
	case "STOP_WORKSPACE_FIRST", "OPEN_NETWORK_FORBIDDEN_IN_SANDBOX":
		shared.Error(w, 409, e.Error())
	case "NOT_READY":
		shared.Error(w, 503, "NOT_READY")
	case "RESOURCE_LIMIT_UNAVAILABLE":
		shared.Error(w, 503, "RESOURCE_LIMIT_UNAVAILABLE")
	case "NETWORK_POLICY_UNAVAILABLE":
		shared.Error(w, 503, "NETWORK_POLICY_UNAVAILABLE")
	case "CONSOLE_UNAVAILABLE", "CONSOLE_NOT_READY":
		shared.Error(w, 503, e.Error())
	case "HERMES_SETUP_REQUIRED":
		shared.Error(w, 409, e.Error())
	default:
		shared.Error(w, 503, "RUNTIME_UNAVAILABLE")
	}
}
func (s *Server) workspace(w http.ResponseWriter, r *http.Request) (shared.Workspace, bool) {
	id := r.PathValue("id")
	if !shared.ValidID(id) {
		shared.Error(w, 400, "INVALID_REQUEST")
		return shared.Workspace{}, false
	}
	ws, e := s.Store.Workspace(r.Context(), id)
	if e != nil {
		shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
		return ws, false
	}
	if !s.AuthorizeProject(w, r, ws.ProjectID) {
		return ws, false
	}
	return ws, true
}
func mutationKey(w http.ResponseWriter, r *http.Request) bool {
	var b struct {
		Key string `json:"idempotency_key"`
	}
	if !shared.Decode(w, r, &b, 1024) {
		return false
	}
	if !shared.ValidKey(b.Key) {
		shared.Error(w, 400, "INVALID_REQUEST")
		return false
	}
	return true
}
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, c := context.WithTimeout(r.Context(), time.Second)
		defer c()
		if s.Store.DB.PingContext(ctx) != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, map[string]string{"status": "ok"})
	})
	secured := http.NewServeMux()
	s.registerConnectionProfiles(secured)
	s.registerToolProfiles(secured)
	s.registerWorkspaceSettings(secured)
	s.registerConsoleUpgrade(secured)
	s.registerDomains(secured)
	s.registerHermes(secured)
	s.registerEvents(secured)
	s.registerHistory(secured)
	s.registerOrchestration(secured)
	secured.HandleFunc("POST /v1/file-handoffs", s.shareFile)
	secured.HandleFunc("/v1/workspaces/{id}/desktop/{path...}", func(w http.ResponseWriter, r *http.Request) {
		ws, ok := s.workspace(w, r)
		if !ok {
			return
		}
		if ws.ProfileID == "" {
			shared.Error(w, 409, "CONSOLE_UPGRADE_REQUIRED")
			return
		}
		proxy, ok := s.Runtime.(desktopRuntime)
		if !ok {
			shared.Error(w, 503, "CONSOLE_UNAVAILABLE")
			return
		}
		proxy.Desktop(w, r, ws.ID, r.PathValue("path"))
	})
	secured.HandleFunc("GET /v1/profiles", func(w http.ResponseWriter, r *http.Request) {
		profiles, err := s.Store.ToolProfiles(r.Context())
		if err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, map[string]any{"items": profiles})
	})
	secured.HandleFunc("/v1/workspaces/{id}/console/{path...}", func(w http.ResponseWriter, r *http.Request) {
		ws, ok := s.workspace(w, r)
		if !ok {
			return
		}
		if ws.ProfileID == "" {
			shared.Error(w, 409, "CONSOLE_UPGRADE_REQUIRED")
			return
		}
		proxy, ok := s.Runtime.(consoleRuntime)
		if !ok {
			shared.Error(w, 503, "CONSOLE_UNAVAILABLE")
			return
		}
		proxy.Console(w, r, ws.ID, r.PathValue("path"))
	})
	secured.HandleFunc("POST /v1/workspaces", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Project             string `json:"project_id"`
			ConnectionProfileID string `json:"connection_profile_id,omitempty"`
			Key                 string `json:"idempotency_key"`
			Name                string `json:"name,omitempty"`
			ProfileID           string `json:"profile_id,omitempty"`
			ParentID            string `json:"parent_id,omitempty"`
			Branch              string `json:"branch,omitempty"`
			AgentKind           string `json:"agent_kind,omitempty"`
			TemplateID          string `json:"template_id,omitempty"`
		}
		if !shared.Decode(w, r, &req, 1024) {
			return
		}
		if req.Project == "" {
			if p, ok := r.Context().Value(principalKey{}).(principal); ok && p.projectID != "" {
				req.Project = p.projectID
			} else {
				req.Project = "default"
			}
		}
		if !s.AuthorizeProject(w, r, req.Project) {
			return
		}
		if req.AgentKind == "" {
			req.AgentKind = "t3"
		}
		if !shared.ValidAgentKind(req.AgentKind) || (req.TemplateID != "" && req.TemplateID != "development-v1") {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		if req.AgentKind == "hermes" {
			c, err := s.Store.HermesSetup(r.Context(), req.Project)
			if err != nil || !s.hermesAvailable(r.Context()) {
				shared.Error(w, 409, "HERMES_SETUP_REQUIRED")
				return
			}
			if req.ConnectionProfileID == "" {
				req.ConnectionProfileID = c.ConnectionProfileID
			}
		}
		if !shared.ValidKey(req.Key) {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		if req.ProfileID == "" {
			req.ProfileID = "general"
		}
		_, profileErr := s.Store.ToolProfile(r.Context(), req.ProfileID)
		if profileErr != nil || len(req.Name) > 80 || strings.ContainsAny(req.Name, "\r\n\x00") || (req.ParentID != "" && !shared.ValidID(req.ParentID)) || (req.Branch != "" && !shared.ValidBranch(req.Branch)) {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		s.createMu.Lock()
		defer s.createMu.Unlock()
		old, e := s.Store.ByProjectKey(r.Context(), req.Project, req.Key)
		if e == nil {
			if old.ProjectID != req.Project {
				shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
				return
			}
			if old.ProfileID != "" && (old.Name != req.Name || old.ProfileID != req.ProfileID || old.ParentID != req.ParentID || old.Branch != req.Branch || old.AgentKind != req.AgentKind || old.TemplateID != req.TemplateID || old.ConnectionProfileID != req.ConnectionProfileID) {
				shared.Error(w, 409, "IDEMPOTENCY_CONFLICT")
				return
			}
			shared.JSON(w, 200, old)
			return
		}
		if e != sql.ErrNoRows {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		if req.ParentID != "" {
			unlock := s.operations.Lock(req.ParentID)
			defer unlock()
			parent, err := s.Store.Workspace(r.Context(), req.ParentID)
			if err != nil || parent.ProjectID != req.Project || (parent.ProfileID != req.ProfileID && !(parent.ProfileID == "" && req.ProfileID == "general")) {
				shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
				return
			}
			if req.Branch == "" {
				shared.Error(w, 400, "BRANCH_REQUIRED")
				return
			}
			if err = s.start(r.Context(), &parent); err != nil {
				apiError(w, err)
				return
			}
			if parent.State != "READY" || parent.ActiveRuns > 0 || parent.BackgroundJobs > 0 {
				shared.Error(w, 409, "WORKSPACE_BUSY")
				return
			}
		}
		all, e := s.Store.Workspaces(r.Context())
		if e != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		if len(all) >= 100 {
			shared.Error(w, 429, "QUOTA_EXCEEDED")
			return
		}
		now := s.now()
		ws := shared.Workspace{ID: shared.NewID("ws_"), ProjectID: req.Project, HomeID: shared.NewID("home_"), AgentKind: req.AgentKind, TemplateID: req.TemplateID, State: "STOPPED", CreatedAt: now, UpdatedAt: now, RuntimeType: "docker", IsolationStatus: "unverified", NetworkStatus: "unverified", CPULimit: 1, MemoryLimitBytes: 512 << 20}
		ws.MemoryLimitBytes = s.cfg.WorkspaceMemory
		ws.ConnectionProfileID = req.ConnectionProfileID
		if err := s.resolveConnectionProfile(r.Context(), &ws); err != nil {
			shared.Error(w, 400, "INVALID_CONNECTION_PROFILE")
			return
		}
		ws.Name, ws.ProfileID, ws.ParentID, ws.Branch = req.Name, req.ProfileID, req.ParentID, req.Branch
		if s.Store.Create(r.Context(), ws, req.Key) != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 201, ws)
	})
	secured.HandleFunc("GET /v1/workspaces", func(w http.ResponseWriter, r *http.Request) {
		items, e := s.Store.Workspaces(r.Context())
		if e != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		project, ok := s.ProjectForRequest(w, r, r.URL.Query().Get("project_id"))
		if !ok {
			return
		}
		filtered := []shared.Workspace{}
		for i := range items {
			if project != "" && items[i].ProjectID != project {
				continue
			}
			unlock := s.operations.Lock(items[i].ID)
			current, err := s.Store.Workspace(r.Context(), items[i].ID)
			if err != nil {
				unlock()
				continue
			}
			items[i] = current
			_ = s.refresh(r.Context(), &items[i])
			s.reportWrite("observe", items[i].ID, s.Store.Save(r.Context(), items[i]))
			filtered = append(filtered, items[i])
			unlock()
		}
		shared.JSON(w, 200, map[string]any{"items": filtered})
	})
	secured.HandleFunc("GET /v1/workspaces/{id}", func(w http.ResponseWriter, r *http.Request) {
		unlock := s.operations.Lock(r.PathValue("id"))
		defer unlock()
		ws, ok := s.workspace(w, r)
		if !ok {
			return
		}
		_ = s.refresh(r.Context(), &ws)
		s.reportWrite("observe", ws.ID, s.Store.Save(r.Context(), ws))
		shared.JSON(w, 200, ws)
	})
	secured.HandleFunc("POST /v1/workspaces/{id}/start", func(w http.ResponseWriter, r *http.Request) {
		if !mutationKey(w, r) {
			return
		}
		unlock := s.operations.Lock(r.PathValue("id"))
		defer unlock()
		ws, ok := s.workspace(w, r)
		if !ok {
			return
		}
		if e := s.resolveConnectionProfile(r.Context(), &ws); e != nil {
			apiError(w, e)
			return
		}
		if e := s.start(r.Context(), &ws); e != nil {
			apiError(w, e)
			return
		}
		shared.JSON(w, 200, ws)
	})
	secured.HandleFunc("POST /v1/workspaces/{id}/stop", func(w http.ResponseWriter, r *http.Request) {
		if !mutationKey(w, r) {
			return
		}
		unlock := s.operations.Lock(r.PathValue("id"))
		defer unlock()
		ws, ok := s.workspace(w, r)
		if !ok {
			return
		}
		if e := s.stop(r.Context(), &ws); e != nil {
			apiError(w, e)
			return
		}
		shared.JSON(w, 200, ws)
	})
	secured.HandleFunc("GET /v1/workspaces/{id}/connection", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.workspace(w, r); ok {
			shared.Error(w, 503, "AGENT_PROTOCOL_UNAVAILABLE")
		}
	})
	secured.HandleFunc("POST /v1/workspaces/{id}/tasks", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Instruction string `json:"instruction"`
			Key         string `json:"idempotency_key"`
			Provider    string `json:"provider,omitempty"`
		}
		if !shared.Decode(w, r, &req, 70<<10) {
			return
		}
		if !shared.ValidKey(req.Key) || req.Instruction == "" || len(req.Instruction) > 64<<10 || (req.Provider != "" && req.Provider != "codex" && req.Provider != "claude" && req.Provider != "hermes") {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		unlock := s.operations.Lock(r.PathValue("id"))
		defer unlock()
		ws, ok := s.workspace(w, r)
		if !ok {
			return
		}
		if ws.AgentKind == "hermes" {
			req.Provider = "hermes"
		} else if req.Provider == "hermes" {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		b, _ := json.Marshal(req)
		if e := s.start(r.Context(), &ws); e != nil {
			apiError(w, e)
			return
		}
		s.forward(w, r, ws.ID, "POST", "/v1/tasks", b, true)
	})
	secured.HandleFunc("GET /v1/workspaces/{id}/tasks/{task}", func(w http.ResponseWriter, r *http.Request) { s.taskRoute(w, r, "", false) })
	secured.HandleFunc("GET /v1/workspaces/{id}/tasks/{task}/result", func(w http.ResponseWriter, r *http.Request) { s.taskRoute(w, r, "/result", false) })
	secured.HandleFunc("POST /v1/workspaces/{id}/tasks/{task}/cancel", func(w http.ResponseWriter, r *http.Request) { s.taskRoute(w, r, "/cancel", true) })
	secured.HandleFunc("GET /v1/usage", func(w http.ResponseWriter, r *http.Request) {
		items, e := s.Store.Workspaces(r.Context())
		if e != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		project, ok := s.ProjectForRequest(w, r, r.URL.Query().Get("project_id"))
		if !ok {
			return
		}
		filtered := []shared.Workspace{}
		for _, ws := range items {
			if project == "" || project == ws.ProjectID {
				filtered = append(filtered, ws)
			}
		}
		shared.JSON(w, 200, s.usage(filtered))
	})
	mux.Handle("/v1/", s.authenticate(secured))
	return mux
}
func (s *Server) taskRoute(w http.ResponseWriter, r *http.Request, suffix string, mutation bool) {
	id := r.PathValue("task")
	if !shared.ValidID(id) {
		shared.Error(w, 400, "INVALID_REQUEST")
		return
	}
	var b []byte
	if mutation {
		var req struct {
			Key string `json:"idempotency_key"`
		}
		if !shared.Decode(w, r, &req, 1024) {
			return
		}
		if !shared.ValidKey(req.Key) {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		b, _ = json.Marshal(req)
	}
	query := ""
	if suffix == "/result" {
		q := url.Values{}
		for k, vs := range r.URL.Query() {
			if (k != "offset" && k != "limit") || len(vs) != 1 {
				shared.Error(w, 400, "INVALID_REQUEST")
				return
			}
			q.Set(k, vs[0])
		}
		if len(q) > 0 {
			query = "?" + q.Encode()
		}
	}
	unlock := s.operations.Lock(r.PathValue("id"))
	defer unlock()
	ws, ok := s.workspace(w, r)
	if !ok {
		return
	}
	if e := s.lifecycle().ObserveTask(r.Context(), &ws); e != nil {
		apiError(w, e)
		return
	}
	s.forward(w, r, ws.ID, r.Method, "/v1/tasks/"+id+suffix+query, b, suffix != "/result")
}
func (s *Server) forward(w http.ResponseWriter, r *http.Request, id, method, path string, b []byte, metadata bool) {
	status, out, e := s.Runtime.Agent(r.Context(), id, method, path, b)
	if e != nil {
		shared.Error(w, 503, "RUNTIME_UNAVAILABLE")
		return
	}
	if status < 200 || status >= 300 {
		var ae shared.APIError
		if json.Unmarshal(out, &ae) != nil || !safeError(ae.Code) {
			shared.Error(w, 503, "WORKSPACE_UNAVAILABLE")
			return
		}
		shared.Error(w, status, ae.Code)
		return
	}
	if metadata {
		var t shared.Task
		if json.Unmarshal(out, &t) != nil || !validTask(t, id) {
			shared.Error(w, 503, "INVALID_WORKSPACE_RESPONSE")
			return
		}
		t.ErrorCode = sanitizedTaskError(t.ErrorCode)
		if s.Store.SaveTask(r.Context(), t) != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.JSON(w, status, t)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(out)
}
func safeError(code string) bool {
	switch code {
	case "INVALID_REQUEST", "NOT_FOUND_OR_FORBIDDEN", "IDEMPOTENCY_CONFLICT", "QUEUE_FULL", "STORAGE_UNAVAILABLE", "WORKSPACE_STOPPING":
		return true
	}
	return false
}
func validTask(t shared.Task, id string) bool { return shared.ValidTask(t, id) }
func sanitizedTaskError(code string) string   { return shared.SanitizedTaskError(code) }
func (s *Server) usage(items []shared.Workspace) map[string]any {
	cpu := 0.0
	var mem int64
	count := 0
	for _, ws := range items {
		if reserved(ws) {
			cpu += ws.CPULimit
			mem += ws.MemoryLimitBytes
			count++
		}
	}
	return map[string]any{"cpu_allocated": cpu, "memory_allocated_bytes": mem, "running_workspaces": count, "max_running_workspaces": s.cfg.MaxRunning, "cpu_quota": s.cfg.CPUQuota, "memory_quota_bytes": s.cfg.MemoryQuota}
}

// Reconcile never starts a workspace. Unknown activity prevents automatic stop.
func (s *Server) Reconcile(ctx context.Context) {
	items, e := s.Store.Workspaces(ctx)
	if e != nil {
		return
	}
	for i := range items {
		unlock := s.operations.Lock(items[i].ID)
		current, err := s.Store.Workspace(ctx, items[i].ID)
		if err != nil {
			unlock()
			continue
		}
		ws := &current
		if e = s.refresh(ctx, ws); e == nil {
			if ws.Ready {
				status, b, err := s.Runtime.Agent(ctx, ws.ID, "GET", "/v1/tasks", nil)
				if err == nil && status == 200 {
					s.captureEvents(ctx, *ws, b)
					var list struct {
						Items []shared.Task `json:"items"`
					}
					if json.Unmarshal(b, &list) == nil {
						for _, t := range list.Items {
							if validTask(t, ws.ID) {
								t.ErrorCode = sanitizedTaskError(t.ErrorCode)
								s.reportWrite("task_observation", t.ID, s.Store.SaveTask(ctx, t))
							}
						}
					}
				}
			}
			if ws.State == "READY" && ws.IdleStopAt != nil && !s.now().Before(*ws.IdleStopAt) {
				_ = s.stop(ctx, ws)
			}
		}
		s.captureEvents(ctx, *ws, nil)
		s.reportWrite("reconcile", ws.ID, s.Store.Save(ctx, *ws))
		unlock()
	}
}
func (s *Server) Run(ctx context.Context) {
	s.probeServices(ctx)
	s.Reconcile(ctx)
	s.tickOrchestration(ctx)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tick, c := context.WithTimeout(ctx, 30*time.Second)
			s.Reconcile(tick)
			s.tickOrchestration(tick)
			s.probeServices(tick)
			c()
		}
	}
}
func (s *Server) probeServices(ctx context.Context) {
	var wg sync.WaitGroup
	client := &http.Client{Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for name, endpoint := range s.cfg.HealthEndpoints {
		wg.Add(1)
		go func(name, endpoint string) {
			defer wg.Done()
			req, e := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
			status := "unavailable"
			reason := "HEALTH_CHECK_FAILED"
			if e == nil {
				resp, err := client.Do(req)
				if err == nil {
					resp.Body.Close()
					if resp.StatusCode == 200 {
						status = "healthy"
						reason = "PROCESS_READY"
					}
				}
			}
			s.mu.Lock()
			s.health[name] = map[string]any{"name": name, "status": status, "reason_code": reason, "observed_at": s.now()}
			s.mu.Unlock()
		}(name, endpoint)
	}
	wg.Wait()
}
func (s *Server) DashboardHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /dashboard-api/v1/snapshot", func(w http.ResponseWriter, r *http.Request) {
		items, e := s.Store.Workspaces(r.Context())
		if e != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		tasks, e := s.Store.Tasks(r.Context())
		if e != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		active := []shared.Task{}
		knownWorkspaces := map[string]shared.Workspace{}
		for _, ws := range items {
			knownWorkspaces[ws.ID] = ws
		}
		for _, t := range tasks {
			if shared.Active(t.State) {
				if ws := knownWorkspaces[t.WorkspaceID]; !ws.Ready || ws.State == "UNKNOWN" {
					t.State = "unknown"
				}
				active = append(active, t)
			}
		}
		now := s.now()
		observed := now
		status := "ok"
		for _, ws := range items {
			if ws.State == "UNKNOWN" {
				status = "partial"
			}
			if ws.ObservedAt != nil && ws.ObservedAt.Before(observed) {
				observed = *ws.ObservedAt
			}
		}
		usage := s.usage(items)
		usage["observed_at"] = observed
		usage["status"] = status
		services := []map[string]any{{"name": "Control plane", "status": "healthy", "observed_at": now}, {"name": "Platform metadata", "status": "healthy", "observed_at": now}}
		serviceStatus := "ok"
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, name := range []string{"Runtime controller", "Inference gateway", "Credential broker", "MCP"} {
			if h, ok := s.health[name]; ok {
				services = append(services, h)
				if h["status"] != "healthy" {
					serviceStatus = "partial"
				}
			} else {
				services = append(services, map[string]any{"name": name, "status": "unknown", "reason_code": "NOT_PROBED", "observed_at": now})
				serviceStatus = "partial"
			}
		}
		services = append(services, map[string]any{"name": "Live provider integration", "status": "unknown", "reason_code": "LIVE_CREDENTIALS_NOT_PROBED", "observed_at": now})
		safeItems := snapshotWorkspaces(items)
		shared.JSON(w, 200, map[string]any{"schema_version": 1, "generated_at": now, "mode": "development", "workspaces": map[string]any{"observed_at": observed, "status": status, "items": safeItems}, "active_tasks": map[string]any{"observed_at": observed, "status": status, "items": active}, "usage": usage, "services": map[string]any{"observed_at": now, "status": serviceStatus, "items": services}})
	})
	return mux
}
