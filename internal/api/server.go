package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/url"
	pathpkg "path"
	"strings"
	"sync"
	"time"

	"agent-workbench/internal/coordination"
	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/runtime/protocol"
	"agent-workbench/internal/shared"
	"agent-workbench/internal/store"
	"agent-workbench/internal/workspacesvc"
)

type Config struct {
	RequireScopedCredentials    bool
	Version                     string
	DashboardReadToken          string
	DashboardLocalOperatorToken string
	HumanSigningKey             string
	ProjectSigningKey           string
	ProjectCreatorEmails        map[string]bool
	MaxProjectsPerUser          int
	CredentialKeys              CredentialKeys
	SubscriptionGatewayToken    string
	RequireAgentSelection       bool
	VerifySubscriptionGrant     func(context.Context, string, string, string) bool
	ProvisionProjectNamespaces  bool
	SharedWorkspaceID           string
	Token                       string
	ProjectTokens               map[string]string
	IdleTimeout                 time.Duration
	MaxRunning                  int
	MaxRunningPerUser           int
	CPUQuota                    float64
	MemoryQuota                 int64
	WorkspaceMemory             int64
	HealthEndpoints             map[string]string
}
type Server struct {
	coordination    *coordination.Service
	schedulerCursor orchestration.Cursor
	workReady       chan struct{}
	eventCache      map[string]*workspaceEventCache
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
	if !c.RequireScopedCredentials && c.DashboardReadToken == "" {
		c.DashboardReadToken = shared.DashboardReadToken(c.Token)
	}
	if !c.RequireScopedCredentials && c.HumanSigningKey == "" {
		c.HumanSigningKey = c.Token
	}
	if !c.RequireScopedCredentials && c.ProjectSigningKey == "" {
		c.ProjectSigningKey = c.Token
	}
	if c.MaxRunning == 0 {
		c.MaxRunning = 4
	}
	if c.SharedWorkspaceID != "" {
		c.MaxRunning = 1
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
	maps.Copy(tokens, c.ProjectTokens)
	c.ProjectTokens = tokens
	server := &Server{Store: s, Runtime: rt, cfg: c, now: func() time.Time { return time.Now().UTC() }, health: map[string]map[string]any{}, workReady: make(chan struct{}, 1)}
	server.coordination = &coordination.Service{DB: s.DB, Blobs: server.projectBlobStore}
	return server
}
func reserved(w shared.Workspace) bool { return workspacesvc.Reserved(w) }
func (s *Server) lifecycle() *workspacesvc.Service {
	return &workspacesvc.Service{Store: s.Store, Runtime: s.Runtime, Config: workspacesvc.Config{IdleTimeout: s.cfg.IdleTimeout, MaxRunning: s.cfg.MaxRunning, MaxRunningPerUser: s.cfg.MaxRunningPerUser, CPUQuota: s.cfg.CPUQuota, MemoryQuota: s.cfg.MemoryQuota, SharedWorkspaceID: s.cfg.SharedWorkspaceID}, Now: s.now, HermesAvailable: s.hermesAvailable, Coordinator: &s.operations, BeforeStop: func(ctx context.Context, ws shared.Workspace) {
		s.syncHistory(ctx, ws, 16, 4*time.Second)
	}}
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

var runtimeUnavailable = shared.CodedError{Status: http.StatusServiceUnavailable, Code: "RUNTIME_UNAVAILABLE"}

func apiError(w http.ResponseWriter, err error) {
	shared.WriteHTTPError(w, err, runtimeUnavailable)
}

func runtimeRequestError(err error) shared.CodedError {
	return shared.ResolveHTTPError(err, runtimeUnavailable)
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
	if s.cfg.SharedWorkspaceID != "" && !s.sharedWorkspace(ws) {
		shared.Error(w, 409, "SHARED_WORKSPACE_ONLY")
		return ws, false
	}
	if s.sharedWorkspace(ws) {
		if _, ok := s.ProjectForRequest(w, r, ""); !ok {
			return ws, false
		}
		ws.Shared = true
	} else if !s.AuthorizeProject(w, r, ws.ProjectID) {
		return ws, false
	}
	return ws, true
}
func mutationKey(w http.ResponseWriter, r *http.Request) bool {
	var b struct {
		Key string `json:"idempotency_key"`
	}
	if !decodeRouteBody(w, r, &b) {
		return false
	}
	if !shared.ValidKey(b.Key) {
		shared.Error(w, 400, "INVALID_REQUEST")
		return false
	}
	return true
}

func workspaceProxy(s *Server, forward func(http.ResponseWriter, *http.Request, string, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ws, ok := s.workspace(w, r)
		if !ok {
			return
		}
		if !s.projectAdmin(w, r, ws.ProjectID) {
			return
		}
		if ws.AgentKind != "t3" && !ws.ConsoleEnabled {
			shared.Error(w, 409, "CONSOLE_UPGRADE_REQUIRED")
			return
		}
		forward(w, r, ws.ID, r.PathValue("path"))
	}
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
	s.registerAgentModels(secured)
	s.registerWorkspaceSettings(secured)
	s.registerConsoleUpgrade(secured)
	s.registerDomains(secured)
	s.registerProjectNamespaces(secured)
	s.registerHermes(secured)
	s.registerEvents(secured)
	s.registerHistory(secured)
	s.registerOrchestration(secured)
	s.registerWork(secured)
	s.registerForges(secured)
	s.registerSubscriptions(secured)
	mux.HandleFunc("POST /internal/subscriptions/resolve", s.resolveSubscription)
	s.registerBoard(secured)
	s.registerCoordinationRuntime(secured)
	s.registerAnalysis(secured)
	s.registerFactoryEnvironments(secured)
	s.registerDeletion(secured)
	s.registerTerminals(secured)
	secured.HandleFunc("GET /v1/workspaces/{id}/setup", s.workerSetup)
	secured.HandleFunc("POST /v1/workspaces/{id}/setup", s.workerSetup)
	secured.HandleFunc("POST /v1/file-handoffs", s.shareFile)
	secured.HandleFunc("/v1/workspaces/{id}/desktop/{path...}", workspaceProxy(s, s.Runtime.Desktop))
	secured.HandleFunc("/v1/workspaces/{id}/console/{path...}", workspaceProxy(s, func(w http.ResponseWriter, r *http.Request, id, path string) {
		route := strings.ToLower(strings.TrimPrefix(pathpkg.Clean("/"+path), "/"))
		if (route == "api/auth" || strings.HasPrefix(route, "api/auth/")) && !(route == "api/auth/session" && r.Method == http.MethodGet) && !s.admin(w, r) {
			return
		}
		s.Runtime.Console(w, r, id, path)
	}))
	secured.HandleFunc("POST /v1/workspaces", func(w http.ResponseWriter, r *http.Request) {
		var req workspaceCreate
		if !decodeRouteBody(w, r, &req) {
			return
		}
		actor, _ := r.Context().Value(principalKey{}).(principal)
		ws, created, err := s.createWorkspace(r.Context(), actor, req)
		if err != nil {
			writeRequestError(w, err)
			return
		}
		shared.JSON(w, createStatus(created), ws)
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
			if (s.cfg.SharedWorkspaceID != "" && !s.sharedWorkspace(items[i])) || (project != "" && !s.workspaceInProject(items[i], project)) {
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
			items[i].Shared = s.sharedWorkspace(items[i])
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
	secured.HandleFunc("GET /v1/workspaces/{id}/start-readiness", func(w http.ResponseWriter, r *http.Request) {
		ws, ok := s.workspace(w, r)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		w.Header().Set("Cache-Control", "no-store")
		shared.JSON(w, 200, s.lifecycle().StartReadiness(ctx, ws))
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
	secured.HandleFunc("POST /v1/workspaces/{id}/force-stop", func(w http.ResponseWriter, r *http.Request) {
		if !s.admin(w, r) || !mutationKey(w, r) {
			return
		}
		unlock := s.operations.Lock(r.PathValue("id"))
		defer unlock()
		ws, ok := s.workspace(w, r)
		if !ok {
			return
		}
		if err := s.lifecycle().ForceStop(r.Context(), &ws); err != nil {
			apiError(w, err)
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
		if !decodeRouteBody(w, r, &req) {
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
		if !decodeRouteBody(w, r, &req) {
			return
		}
		if !shared.ValidKey(req.Key) {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		b, _ = json.Marshal(req)
	}
	query := ""
	noStart := false
	if suffix == "/result" {
		q := url.Values{}
		for k, vs := range r.URL.Query() {
			if k == "no_start" && len(vs) == 1 && vs[0] == "1" {
				noStart = true
				continue
			}
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
	if noStart {
		if err := s.refresh(r.Context(), &ws); err != nil {
			apiError(w, err)
			return
		}
		if !ws.Ready {
			shared.Error(w, 409, "WORKSPACE_STOPPED")
			return
		}
	} else if e := s.lifecycle().ObserveTask(r.Context(), &ws); e != nil {
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
		if s.cfg.SharedWorkspaceID != "" && !s.sharedWorkspace(items[i]) {
			continue
		}
		unlock := s.operations.Lock(items[i].ID)
		current, err := s.Store.Workspace(ctx, items[i].ID)
		if err != nil {
			unlock()
			continue
		}
		ws := &current
		if e = s.refresh(ctx, ws); e == nil {
			if ws.Ready {
				s.syncHistory(ctx, *ws, 4, time.Second)
				status, b, err := s.Runtime.Agent(ctx, ws.ID, "GET", "/v1/tasks", nil)
				if err == nil && status == 200 {
					s.captureEvents(ctx, *ws, b)
					var list struct {
						Items []shared.Task `json:"items"`
					}
					if json.Unmarshal(b, &list) == nil {
						tasks := list.Items[:0]
						for _, t := range list.Items {
							if validTask(t, ws.ID) {
								t.ErrorCode = sanitizedTaskError(t.ErrorCode)
								tasks = append(tasks, t)
							}
						}
						s.reportWrite("task_observation", ws.ID, s.Store.SaveObservedTasks(ctx, ws.ID, tasks))
					}
				}
			}
			s.reconcileRetainedPreview(ctx, ws)
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
	var scheduler sync.WaitGroup
	scheduler.Go(func() { s.runOrchestration(ctx) })
	defer scheduler.Wait()

	s.probeServices(ctx)
	s.Reconcile(ctx)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tick, c := context.WithTimeout(ctx, 30*time.Second)
			s.Reconcile(tick)
			s.probeServices(tick)
			c()
		}
	}
}
func (s *Server) probeServices(ctx context.Context) {
	var wg sync.WaitGroup
	client := &http.Client{Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for name, endpoint := range s.cfg.HealthEndpoints {
		wg.Go(func() {
			req, e := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
			status := "unavailable"
			reason := "HEALTH_CHECK_FAILED"
			if e == nil {
				resp, err := client.Do(req)
				if err == nil {
					// Health handlers return short bodies. Read to EOF when
					// bounded so the next probe can reuse the connection.
					_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
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
		})
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
		if s.cfg.SharedWorkspaceID != "" {
			selected := []shared.Workspace{}
			for _, ws := range items {
				if s.sharedWorkspace(ws) {
					ws.Shared = true
					selected = append(selected, ws)
				}
			}
			items = selected
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
		safeItems := snapshotWorkspaces(items)
		shared.JSON(w, 200, map[string]any{"schema_version": 1, "version": s.cfg.Version, "generated_at": now, "mode": "development", "workspaces": map[string]any{"observed_at": observed, "status": status, "items": safeItems}, "active_tasks": map[string]any{"observed_at": observed, "status": status, "items": active}, "usage": usage, "services": map[string]any{"observed_at": now, "status": serviceStatus, "items": services}})
	})
	return shared.RequireToken(s.cfg.DashboardReadToken, mux)
}

type workspaceCreate struct {
	Project             string `json:"project_id"`
	ConnectionProfileID string `json:"connection_profile_id,omitempty"`
	Key                 string `json:"idempotency_key"`
	Name                string `json:"name,omitempty"`
	ParentID            string `json:"parent_id,omitempty"`
	Branch              string `json:"branch,omitempty"`
	AgentKind           string `json:"agent_kind,omitempty"`
	TemplateID          string `json:"template_id,omitempty"`
}

func (s *Server) createWorkspace(ctx context.Context, actor principal, req workspaceCreate) (shared.Workspace, bool, error) {
	if req.Project == "" {
		if actor.projectID != "" {
			req.Project = actor.projectID
		} else {
			req.Project = "default"
		}
	}
	if err := s.authorizeProject(ctx, actor, req.Project); err != nil {
		return shared.Workspace{}, false, err
	}
	if s.cfg.SharedWorkspaceID != "" {
		if !shared.ValidKey(req.Key) {
			return shared.Workspace{}, false, shared.CodedError{Status: http.StatusBadRequest, Code: "INVALID_REQUEST"}
		}
		if req.ParentID != "" || req.Branch != "" || (req.AgentKind != "" && req.AgentKind != "t3") || (req.TemplateID != "" && req.TemplateID != "development-v1") {
			return shared.Workspace{}, false, shared.CodedError{Status: http.StatusConflict, Code: "SHARED_WORKSPACE_ONLY"}
		}
		s.createMu.Lock()
		defer s.createMu.Unlock()
		ws, err := s.EnsureSharedWorkspace(ctx)
		if err != nil {
			return shared.Workspace{}, false, shared.CodedError{Status: http.StatusServiceUnavailable, Code: "STORAGE_UNAVAILABLE"}
		}
		if req.ConnectionProfileID != "" && req.ConnectionProfileID != ws.ConnectionProfileID {
			return shared.Workspace{}, false, shared.CodedError{Status: http.StatusConflict, Code: "SHARED_WORKSPACE_ONLY"}
		}
		return ws, false, nil
	}
	if req.AgentKind == "" {
		req.AgentKind = "t3"
	}
	if !shared.ValidAgentKind(req.AgentKind) || (req.TemplateID != "" && req.TemplateID != "development-v1") {
		return shared.Workspace{}, false, shared.CodedError{Status: http.StatusBadRequest, Code: "INVALID_REQUEST"}
	}
	if req.AgentKind == "hermes" {
		c, err := s.Store.HermesSetup(ctx, req.Project)
		if err != nil || !s.hermesAvailable(ctx) {
			return shared.Workspace{}, false, shared.CodedError{Status: http.StatusConflict, Code: "HERMES_SETUP_REQUIRED"}
		}
		if req.ConnectionProfileID == "" {
			req.ConnectionProfileID = c.ConnectionProfileID
		}
	}
	if !shared.ValidKey(req.Key) {
		return shared.Workspace{}, false, shared.CodedError{Status: http.StatusBadRequest, Code: "INVALID_REQUEST"}
	}
	if len(req.Name) > 80 || strings.ContainsAny(req.Name, "\r\n\x00") || (req.ParentID != "" && !shared.ValidID(req.ParentID)) || (req.Branch != "" && !shared.ValidBranch(req.Branch)) {
		return shared.Workspace{}, false, shared.CodedError{Status: http.StatusBadRequest, Code: "INVALID_REQUEST"}
	}
	s.createMu.Lock()
	defer s.createMu.Unlock()
	if err := s.authorizeProject(ctx, actor, req.Project); err != nil {
		return shared.Workspace{}, false, err
	}
	old, e := s.Store.ByProjectKey(ctx, req.Project, req.Key)
	if e == nil {
		if old.ProjectID != req.Project {
			return shared.Workspace{}, false, shared.CodedError{Status: http.StatusNotFound, Code: "NOT_FOUND_OR_FORBIDDEN"}
		}
		if old.Name != req.Name || old.ParentID != req.ParentID || old.Branch != req.Branch || old.AgentKind != req.AgentKind || old.TemplateID != req.TemplateID || old.ConnectionProfileID != req.ConnectionProfileID {
			return shared.Workspace{}, false, shared.CodedError{Status: http.StatusConflict, Code: "IDEMPOTENCY_CONFLICT"}
		}
		return old, false, nil
	}
	if e != sql.ErrNoRows {
		return shared.Workspace{}, false, shared.CodedError{Status: http.StatusServiceUnavailable, Code: "STORAGE_UNAVAILABLE"}
	}
	if req.ParentID != "" {
		unlock := s.operations.Lock(req.ParentID)
		defer unlock()
		parent, err := s.Store.Workspace(ctx, req.ParentID)
		if err != nil || parent.ProjectID != req.Project {
			return shared.Workspace{}, false, shared.CodedError{Status: http.StatusNotFound, Code: "NOT_FOUND_OR_FORBIDDEN"}
		}
		if req.Branch == "" {
			return shared.Workspace{}, false, shared.CodedError{Status: http.StatusBadRequest, Code: "BRANCH_REQUIRED"}
		}
		if err = s.start(ctx, &parent); err != nil {
			return shared.Workspace{}, false, runtimeRequestError(err)
		}
		if parent.State != "READY" || parent.ActiveRuns > 0 || parent.BackgroundJobs > 0 {
			return shared.Workspace{}, false, shared.CodedError{Status: http.StatusConflict, Code: "WORKSPACE_BUSY"}
		}
	}
	all, e := s.Store.Workspaces(ctx)
	if e != nil {
		return shared.Workspace{}, false, shared.CodedError{Status: http.StatusServiceUnavailable, Code: "STORAGE_UNAVAILABLE"}
	}
	if len(all) >= 100 {
		return shared.Workspace{}, false, shared.CodedError{Status: http.StatusTooManyRequests, Code: "QUOTA_EXCEEDED"}
	}
	now := s.now()
	ws := shared.Workspace{ID: shared.NewID("ws_"), ProjectID: req.Project, HomeID: shared.NewID("home_"), AgentKind: req.AgentKind, TemplateID: req.TemplateID, State: "STOPPED", CreatedAt: now, UpdatedAt: now, RuntimeType: "docker", IsolationStatus: "unverified", NetworkStatus: "unverified", CPULimit: 1, MemoryLimitBytes: 512 << 20}
	ws.MemoryLimitBytes = s.cfg.WorkspaceMemory
	ws.ConnectionProfileID = req.ConnectionProfileID
	if err := s.resolveConnectionProfile(ctx, &ws); err != nil {
		return shared.Workspace{}, false, shared.CodedError{Status: http.StatusBadRequest, Code: "INVALID_CONNECTION_PROFILE"}
	}
	ws.Name, ws.ParentID, ws.Branch = req.Name, req.ParentID, req.Branch
	if s.Store.Create(ctx, ws, req.Key) != nil {
		return shared.Workspace{}, false, shared.CodedError{Status: http.StatusServiceUnavailable, Code: "STORAGE_UNAVAILABLE"}
	}
	return ws, true, nil
}
