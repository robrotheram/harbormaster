package workspace

import (
	"agent-workbench/internal/workspace/nativeagent"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"agent-workbench/internal/egress"
	"agent-workbench/internal/shared"
	"agent-workbench/internal/workspace/taskstore"
	_ "modernc.org/sqlite"
	"net"
)

type Config struct {
	HermesMCPTransport nativeagent.MCPTransport

	HermesMatrix                                       *shared.HermesMatrix
	HermesModel                                        string
	ConnectionMode                                     string
	EgressURL                                          string
	ProjectID, HomeID, AgentKind                       string
	ID, DataDir, Token, InferenceURL, Model, BrokerURL string
	DefaultProvider, CodexModel, ClaudeModel           string
	Console                                            bool
	Branch                                             string
	Name                                               string
	MonitorProcesses                                   bool
	HTTP                                               *http.Client
}
type Manager struct {
	terminals        map[string]*terminalSession
	applications     map[string]*applicationProcess
	historyCollectMu sync.Mutex
	historyStop      chan struct{}
	historyT3Seen    map[string]string
	historyT3Offset  int
	eventMu          sync.Mutex
	eventWindow      time.Time
	eventCount       int
	observedT3       map[string]string
	matrix           *matrixProcess
	egressProxy      *http.Server
	ownerLock        *os.File
	cfg              Config
	db               *sql.DB
	tasks            *taskstore.Repository
	root             *os.Root
	mu               sync.Mutex
	serial           sync.Mutex
	cancels          map[string]context.CancelFunc
	wg               sync.WaitGroup
	lastActivity     time.Time
	closed           bool
	quiescing        bool
	console          *console
}
type submit struct {
	Model               string `json:"model,omitempty"`
	SpecificationDigest string `json:"-"`
	SourcePath          string `json:"-"`
	Instruction         string `json:"instruction"`
	Key                 string `json:"idempotency_key"`
	Provider            string `json:"provider,omitempty"`
}
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func New(cfg Config) (*Manager, error) {
	if cfg.ID == "" || cfg.Token == "" || cfg.DataDir == "" {
		return nil, fmt.Errorf("workspace configuration missing")
	}
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	lock, e := initializeHome(cfg)
	if e != nil {
		return nil, e
	}
	owned := false
	defer func() {
		if !owned {
			lock.Close()
		}
	}()
	if err := os.MkdirAll(filepath.Join(cfg.DataDir, "files"), 0700); err != nil {
		return nil, err
	}
	home := cfg.DataDir
	if cfg.HomeID != "" {
		home = filepath.Join(cfg.DataDir, "homes", cfg.HomeID)
	}
	if e = os.MkdirAll(home, 0700); e != nil {
		return nil, e
	}
	db, e := sql.Open("sqlite", filepath.Join(home, "workspace.db"))
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	for _, q := range []string{`PRAGMA journal_mode=WAL`, `PRAGMA busy_timeout=5000`,
		`CREATE TABLE IF NOT EXISTS agent_history(seq INTEGER PRIMARY KEY AUTOINCREMENT,id TEXT UNIQUE NOT NULL,episode_id TEXT NOT NULL,body TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS agent_history_episode ON agent_history(episode_id)`,
		`CREATE TABLE IF NOT EXISTS history_state(id INTEGER PRIMARY KEY,bytes_used INTEGER NOT NULL DEFAULT 0,limited INTEGER NOT NULL DEFAULT 0)`,
		`INSERT INTO history_state(id) VALUES(1) ON CONFLICT(id) DO NOTHING`,
		`CREATE TABLE IF NOT EXISTS activity_events(id TEXT PRIMARY KEY,at TEXT NOT NULL,body TEXT NOT NULL)`,
	} {
		if _, e = db.Exec(q); e != nil {
			db.Close()
			return nil, e
		}
	}
	tasks := taskstore.New(db, cfg.ID)
	if err := tasks.Init(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	root, e := os.OpenRoot(filepath.Join(cfg.DataDir, "files"))
	if e != nil {
		db.Close()
		return nil, e
	}
	m := &Manager{historyStop: make(chan struct{}), cfg: cfg, db: db, tasks: tasks, root: root, ownerLock: lock, cancels: map[string]context.CancelFunc{}, lastActivity: time.Now().UTC()}
	if err := m.initFactory(); err != nil {
		root.Close()
		db.Close()
		return nil, err
	}
	if cfg.EgressURL != "" {
		listener, err := net.Listen("tcp", "127.0.0.1:8099")
		if err != nil {
			root.Close()
			db.Close()
			return nil, err
		}
		m.egressProxy = &http.Server{Handler: egress.Handler(nil, cfg.EgressURL, cfg.Token), ReadHeaderTimeout: 5 * time.Second}
		go m.egressProxy.Serve(listener)
	}
	_, consoleErr := os.Stat(filepath.Join(home, ".t3-enabled"))
	if cfg.Console || consoleErr == nil {
		if err := m.startConsole(); err != nil {
			if m.egressProxy != nil {
				m.egressProxy.Close()
			}
			root.Close()
			db.Close()
			return nil, err
		}
	}
	owned = true
	m.wg.Add(1)
	go m.collectHistory()
	m.wg.Add(1)
	go m.expireServices()
	return m, nil
}
func (m *Manager) Close() error {
	m.stopMatrix()
	if m.egressProxy != nil {
		m.egressProxy.Close()
	}
	m.mu.Lock()
	if !m.closed {
		close(m.historyStop)
	}
	m.closed = true
	for _, terminal := range m.terminals {
		terminal.stop()
	}
	for id := range m.applications {
		m.stopApplication(id)
	}
	for _, c := range m.cancels {
		c()
	}
	m.mu.Unlock()
	m.wg.Wait()
	if m.console != nil {
		_ = syscall.Kill(-m.console.cmd.Process.Pid, syscall.SIGTERM)
		select {
		case <-m.console.done:
		case <-time.After(3 * time.Second):
			_ = syscall.Kill(-m.console.cmd.Process.Pid, syscall.SIGKILL)
		}
	}
	m.root.Close()
	m.ownerLock.Close()
	return m.db.Close()
}
func (m *Manager) task(id string) (shared.Task, error) { return m.tasks.Get(context.Background(), id) }
func (m *Manager) submit(s submit) (shared.Task, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.submitLocked(s)
}

func (m *Manager) submitLocked(s submit) (shared.Task, int, error) {
	if s.Model != "" && !shared.ValidWorkSelection(s.Provider, s.Model) {
		return shared.Task{}, 400, shared.CodedError{Status: http.StatusBadRequest, Code: "INVALID_REQUEST"}
	}
	if m.closed || m.quiescing {
		return shared.Task{}, 503, shared.CodedError{Status: http.StatusServiceUnavailable, Code: "WORKSPACE_STOPPING"}
	}
	if s.Provider == "" {
		s.Provider = m.cfg.DefaultProvider
	}
	if s.Provider != "" && s.Provider != "codex" && s.Provider != "claude" && !(s.Provider == "hermes" && m.cfg.AgentKind == "hermes") {
		return shared.Task{}, 400, shared.CodedError{Status: http.StatusBadRequest, Code: "INVALID_REQUEST"}
	}
	var jobs int
	if err := m.db.QueryRow(`SELECT count(*) FROM recipe_jobs WHERE state IN ('running','cancel_requested')`).Scan(&jobs); err != nil {
		return shared.Task{}, 503, shared.CodedError{Status: http.StatusServiceUnavailable, Code: "STORAGE_UNAVAILABLE"}
	}
	if jobs > 0 {
		return shared.Task{}, 409, shared.CodedError{Status: http.StatusConflict, Code: "WORKSPACE_BUSY"}
	}
	now := time.Now().UTC()
	accepted, err := m.tasks.Accept(context.Background(), taskstore.Submission{Instruction: s.Instruction, Key: s.Key, Provider: s.Provider, Model: s.Model, SourcePath: s.SourcePath, SpecificationDigest: s.SpecificationDigest}, now)
	if err != nil {
		if errors.Is(err, taskstore.ErrConflict) {
			return shared.Task{}, 409, err
		}
		if errors.Is(err, taskstore.ErrQueueFull) {
			return shared.Task{}, 429, err
		}
		return shared.Task{}, 503, shared.CodedError{Status: http.StatusServiceUnavailable, Code: "STORAGE_UNAVAILABLE"}
	}
	if !accepted.Created {
		return accepted.Task, 200, nil
	}
	id := accepted.Task.ID
	ctx, cancel := context.WithCancel(context.Background())
	m.cancels[id] = cancel
	m.lastActivity = now
	m.wg.Add(1)
	source := s.Provider
	if source == "" {
		source = "workspace"
	}
	m.history(shared.HistoryRecord{ID: stableHistoryID(id, "user.message", "prompt"), EpisodeID: id, Source: source, Kind: "user.message", Role: "user", Content: s.Instruction})
	m.event("workspace", "task.queued", id, "", "")
	go m.execute(ctx, id, s.Instruction, s.Provider, s.Model)
	return accepted.Task, 202, nil
}
func (m *Manager) execute(ctx context.Context, id, instruction, provider string, models ...string) {
	defer m.wg.Done()
	m.serial.Lock()
	defer m.serial.Unlock()
	if ctx.Err() == nil {
		_ = m.tasks.Start(ctx, id, time.Now().UTC())
	}
	started := time.Now()
	source := provider
	if source == "" {
		source = "workspace"
	}
	m.history(shared.HistoryRecord{EpisodeID: id, Source: source, Kind: "run.started", Status: "running"})
	m.event("workspace", "task.started", id, "", "")
	var result, code string
	if provider == "" {
		result, code = m.run(ctx, id, instruction)
	} else {
		result, code = m.runNativeTask(ctx, provider, instruction, id, models...)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	state := "succeeded"
	if code != "" {
		state = "failed"
	}
	if ctx.Err() != nil {
		state = "cancelled"
		code = ""
	}
	// If Finish fails, the non-terminal durable row prevents unsafe idle stop.
	_ = m.tasks.Finish(context.Background(), id, state, result, code, time.Now().UTC())
	if cancel := m.cancels[id]; cancel != nil {
		cancel()
		delete(m.cancels, id)
	}
	m.historyTaskResult(ctx, id, provider, result, code, state, time.Since(started))
	m.event("workspace", "task."+state, id, "", "")
	m.lastActivity = time.Now().UTC()
}
func (m *Manager) run(ctx context.Context, id, instruction string) (string, string) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	messages := []Message{{Role: "system", Content: "You are a workspace coding agent. Work only on the user's request. Files are relative to the workspace. Use the available tools to inspect, edit and test. Do not expose credentials. Commands have a 30 second limit and background processes are unsupported."}, {Role: "user", Content: instruction}}
	for turn := 0; turn < 16; turn++ {
		if ctx.Err() != nil {
			return "", "EXECUTION_INTERRUPTED"
		}
		b, _ := json.Marshal(map[string]any{"model": m.cfg.Model, "messages": messages, "tools": toolDefinitions(), "stream": false})
		r, e := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(m.cfg.InferenceURL, "/")+"/v1/chat/completions", bytes.NewReader(b))
		if e != nil {
			return "", "INFERENCE_UNAVAILABLE"
		}
		r.Header.Set("Authorization", "Bearer "+m.cfg.Token)
		r.Header.Set("Content-Type", "application/json")
		resp, e := m.cfg.HTTP.Do(r)
		if e != nil {
			return "", "INFERENCE_UNAVAILABLE"
		}
		body, e := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
		resp.Body.Close()
		if e != nil || len(body) > 1<<20 || resp.StatusCode != 200 {
			return "", "INFERENCE_REJECTED"
		}
		var out struct {
			Choices []struct {
				Message Message `json:"message"`
			} `json:"choices"`
		}
		if json.Unmarshal(body, &out) != nil || len(out.Choices) != 1 {
			return "", "INVALID_INFERENCE_RESPONSE"
		}
		msg := out.Choices[0].Message
		msg.Role = "assistant"
		messages = append(messages, msg)
		if len(msg.ToolCalls) > 8 {
			return "", "TOOL_LIMIT"
		}
		if len(msg.ToolCalls) == 0 {
			if len(msg.Content) > 1<<20 {
				return "", "RESULT_TOO_LARGE"
			}
			if !m.saveMessages(id, messages) {
				return "", "STORAGE_UNAVAILABLE"
			}
			return msg.Content, ""
		}
		for _, call := range msg.ToolCalls {
			m.history(shared.HistoryRecord{EpisodeID: id, Source: "workspace", Kind: "tool.call", Role: "assistant", Tool: call.Function.Name, ToolCallID: call.ID, Content: call.Function.Arguments})
			m.event("workspace", "tool.started", id, shared.ActivityTool(call.Function.Name), "")
			result := m.tool(ctx, call.Function.Name, json.RawMessage(call.Function.Arguments))
			m.history(shared.HistoryRecord{EpisodeID: id, Source: "workspace", Kind: "tool.result", Role: "tool", Tool: call.Function.Name, ToolCallID: call.ID, Content: result})
			m.event("workspace", "tool.completed", id, shared.ActivityTool(call.Function.Name), "")
			messages = append(messages, Message{Role: "tool", ToolCallID: call.ID, Content: result})
		}
		if !m.saveMessages(id, messages) {
			return "", "STORAGE_UNAVAILABLE"
		}
	}
	return "", "TURN_LIMIT"
}
func (m *Manager) saveMessages(id string, messages []Message) bool {
	b, e := json.Marshal(messages)
	if e != nil || len(b) > 2<<20 {
		return false
	}
	e = m.tasks.SaveMessages(context.Background(), id, b)
	return e == nil
}
func (m *Manager) Activity() shared.Activity {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activityLocked()
}
func (m *Manager) activityLocked() shared.Activity { return m.activitySnapshotLocked(false) }

// Managed previews may run beside agent work, but still prevent idle shutdown.
func (m *Manager) workActivityLocked() shared.Activity { return m.activitySnapshotLocked(true) }
func (m *Manager) activitySnapshotLocked(allowApplications bool) shared.Activity {
	a := shared.Activity{Known: true, LastActivity: m.lastActivity}
	for _, terminal := range m.terminals {
		terminal.mu.Lock()
		if !terminal.exited {
			a.BackgroundJobs++
		}
		terminal.mu.Unlock()
	}
	var recipes int
	if err := m.db.QueryRow(`SELECT count(*) FROM recipe_jobs WHERE state IN ('running','cancel_requested')`).Scan(&recipes); err != nil {
		a.Known = false
	}
	a.BackgroundJobs += recipes
	var services int
	query := `SELECT count(*) FROM retained_services WHERE state='active' AND expires_at>?`
	if allowApplications {
		query += ` AND id NOT IN (SELECT service_id FROM workspace_applications)`
	}
	if err := m.db.QueryRow(query, time.Now().Unix()).Scan(&services); err != nil {
		a.Known = false
	}
	a.BackgroundJobs += services
	if m.console != nil {
		c := m.consoleActivity()
		a.Known = c.Known
		a.BackgroundJobs += c.BackgroundJobs
		if c.LastActivity.After(a.LastActivity) {
			a.LastActivity = c.LastActivity
		}
		a.ActiveRuns = c.ActiveRuns
	}
	taskRuns, err := m.tasks.ActiveCount(context.Background())
	if err != nil {
		a.Known = false
	}
	a.ActiveRuns += taskRuns
	if m.cfg.MonitorProcesses {
		kernelParent := false
		if stat, err := os.ReadFile("/proc/2/stat"); err == nil {
			kernelParent = kernelInfrastructure(string(stat))
		}
		entries, e := os.ReadDir("/proc")
		if e != nil {
			a.Known = false
		} else {
			for _, entry := range entries {
				pid, e := strconv.Atoi(entry.Name())
				if e == nil && pid != os.Getpid() {
					if pid == 1 && os.Getppid() == 1 {
						if comm, err := os.ReadFile("/proc/1/comm"); err == nil && (strings.TrimSpace(string(comm)) == "docker-init" || strings.TrimSpace(string(comm)) == "awb-init") {
							continue
						}
						if exe, err := os.Readlink("/proc/1/exe"); err == nil && (exe == "/usr/sbin/awb-init" || exe == "/sbin/awb-init") {
							continue
						}
					}
					stat, err := os.ReadFile("/proc/" + entry.Name() + "/stat")
					if os.IsNotExist(err) {
						continue
					}
					if err != nil {
						a.Known = false
						continue
					}
					// Zombies cannot execute or block shutdown. New containers use
					// Docker's init process to reap orphaned native CLI descendants.
					if processState(string(stat)) == "Z" || kernelInfrastructure(string(stat)) || kernelChild(string(stat), kernelParent) {
						continue
					}
					if allowApplications && m.ownedApplicationProcess(pid, string(stat)) {
						continue
					}
					if m.consoleInfrastructure(pid, string(stat)) {
						continue
					}
					a.BackgroundJobs++
				}
			}
		}
	}
	return a
}

// A VM exposes its own kernel threads in /proc; containers did not. Kernel
// workers and kernel-created usermode helpers are not detached agent jobs.
// Use kernel-maintained task flags, never a guest-controlled process name.
func kernelInfrastructure(stat string) bool {
	i := strings.LastIndex(stat, ") ")
	if i < 0 {
		return false
	}
	fields := strings.Fields(stat[i+2:])
	if len(fields) < 7 {
		return false
	}
	flags, e := strconv.ParseUint(fields[6], 10, 64)
	return e == nil && flags&(0x00200000|0x02000000) != 0
}

func kernelChild(stat string, kernelParent bool) bool {
	if !kernelParent {
		return false
	}
	i := strings.LastIndex(stat, ") ")
	if i < 0 {
		return false
	}
	fields := strings.Fields(stat[i+2:])
	return len(fields) >= 2 && fields[1] == "2"
}

func processState(stat string) string {
	i := strings.LastIndex(stat, ") ")
	if i < 0 || len(stat) <= i+2 {
		return ""
	}
	fields := strings.Fields(stat[i+2:])
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}
func (m *Manager) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { shared.JSON(w, 200, map[string]string{"status": "ok"}) })
	api := http.NewServeMux()
	api.HandleFunc("POST /v1/terminals", m.createTerminal)
	api.HandleFunc("POST /v1/terminals/{terminal}/{action}", m.terminalRoute)
	api.HandleFunc("GET /v1/runtime-capabilities", m.runtimeCapabilities)
	api.HandleFunc("POST /v1/matrix/start", m.startMatrix)
	api.HandleFunc("POST /v1/matrix/stop", func(w http.ResponseWriter, r *http.Request) {
		m.stopMatrix()
		shared.JSON(w, 200, map[string]string{"state": "disconnected"})
	})
	api.HandleFunc("GET /v1/matrix/status", m.matrixStatus)
	api.HandleFunc("POST /v1/console/enable", m.enableConsole)
	api.HandleFunc("GET /v1/models", m.models)
	api.HandleFunc("POST /v1/tasks/{id}/context", m.conversationContext)
	api.HandleFunc("POST /v1/files/read", m.handoff)
	api.HandleFunc("POST /v1/files/write-new", m.handoff)
	api.HandleFunc("/v1/t3/{path...}", m.consoleProxy)
	api.HandleFunc("/v1/t3-desktop/{path...}", m.desktopProxy)
	api.HandleFunc("POST /v1/quiesce", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		a := m.activityLocked()
		if !a.Known || a.ActiveRuns > 0 || a.BackgroundJobs > 0 {
			m.mu.Unlock()
			shared.Error(w, 409, "WORKSPACE_BUSY")
			return
		}
		m.quiescing = true
		m.mu.Unlock()
		// collectHistory holds this lock for a full pass and checks quiescing
		// under m.mu after acquiring it. Wait for any pass already in flight.
		m.historyCollectMu.Lock()
		//lint:ignore SA2001 This lock pair is a quiesce barrier for history collection.
		m.historyCollectMu.Unlock()
		shared.JSON(w, 200, map[string]bool{"quiesced": true})
	})
	api.HandleFunc("POST /v1/resume", func(w http.ResponseWriter, r *http.Request) {
		if m.console != nil {
			if _, err := m.console.session(r.Context()); err != nil {
				shared.Error(w, 503, "CONSOLE_NOT_READY")
				return
			}
			if !m.consoleActivity().Known {
				shared.Error(w, 503, "CONSOLE_NOT_READY")
				return
			}
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.quiescing {
			m.quiescing = false
			m.lastActivity = time.Now().UTC()
		}
		shared.JSON(w, 200, map[string]bool{"ready": true})
	})
	api.HandleFunc("GET /v1/activity", func(w http.ResponseWriter, r *http.Request) { shared.JSON(w, 200, m.Activity()) })
	api.HandleFunc("POST /v1/tasks", func(w http.ResponseWriter, r *http.Request) {
		var s submit
		if !shared.Decode(w, r, &s, 70<<10) {
			return
		}
		if !shared.ValidKey(s.Key) || len(s.Instruction) == 0 || len(s.Instruction) > 64<<10 {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		t, status, e := m.submit(s)
		if e != nil {
			shared.WriteHTTPError(w, e, shared.CodedError{Status: status, Code: e.Error()})
			return
		}
		shared.JSON(w, status, t)
	})
	api.HandleFunc("POST /v1/tasks/from-file", m.submitFile)
	api.HandleFunc("POST /v1/artifacts", m.publishArtifact)
	api.HandleFunc("GET /v1/artifacts", m.listArtifacts)
	api.HandleFunc("GET /v1/artifacts/{id}/content", m.artifactContent)
	m.registerCoordination(api)
	api.HandleFunc("POST /v1/services/receipt", m.serviceReceipt)
	api.HandleFunc("POST /v1/services", m.createService)
	api.HandleFunc("GET /v1/services", m.listApplications)
	api.HandleFunc("GET /v1/ports", m.listPorts)
	api.HandleFunc("/v1/application-proxy/{service}/{path...}", m.proxyApplication)
	api.HandleFunc("GET /v1/services/{id}", m.getService)
	api.HandleFunc("POST /v1/services/{id}/revoke", m.revokeService)
	api.HandleFunc("POST /v1/services/{id}/fetch", m.fetchService)
	api.HandleFunc("POST /v1/services/reconcile", m.reconcileServices)
	api.HandleFunc("GET /v1/recipe-jobs", m.listRecipes)
	api.HandleFunc("POST /v1/recipe-jobs", m.startRecipe)
	api.HandleFunc("GET /v1/recipe-jobs/{id}", m.getRecipe)
	api.HandleFunc("POST /v1/recipe-jobs/{id}/cancel", m.cancelRecipe)
	api.HandleFunc("GET /v1/tasks", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("history") == "1" {
			m.historyPage(w, r)
			return
		}
		items, e := m.tasks.List(r.Context())
		if e != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, map[string]any{"items": items, "events": m.events(), "events_supported": true})
	})
	api.HandleFunc("GET /v1/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		t, e := m.task(r.PathValue("id"))
		if e != nil {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		shared.JSON(w, 200, t)
	})
	api.HandleFunc("POST /v1/tasks/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
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
		m.mu.Lock()
		t, e := m.tasks.RequestCancel(r.Context(), r.PathValue("id"), time.Now().UTC())
		if e == nil && shared.Active(t.State) {
			if c := m.cancels[t.ID]; c != nil {
				c()
			}
		}
		m.mu.Unlock()
		if e != nil {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		shared.JSON(w, 200, t)
	})
	api.HandleFunc("GET /v1/tasks/{id}/result", func(w http.ResponseWriter, r *http.Request) {
		state, result, e := m.tasks.Result(r.Context(), r.PathValue("id"))
		if e != nil {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		offset := 0
		limit := 65536
		var er error
		if s := r.URL.Query().Get("offset"); s != "" {
			offset, er = strconv.Atoi(s)
		}
		if er != nil || offset < 0 || offset > len(result) || (offset < len(result) && !utf8.RuneStart(result[offset])) {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		if s := r.URL.Query().Get("limit"); s != "" {
			limit, er = strconv.Atoi(s)
		}
		if er != nil || limit < 1 || limit > 65536 {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		end := min(offset+limit, len(result))
		for end < len(result) && end > offset && !utf8.RuneStart(result[end]) {
			end--
		}
		if end == offset && offset < len(result) {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		next := -1
		if end < len(result) {
			next = end
		}
		shared.JSON(w, 200, map[string]any{"task_id": r.PathValue("id"), "state": state, "content": result[offset:end], "next_offset": next})
	})
	mux.Handle("/v1/", shared.RequireToken(m.cfg.Token, api))
	return mux
}
