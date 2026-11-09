package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"agent-workbench/internal/egress"
	"agent-workbench/internal/shared"
	"agent-workbench/internal/workspace/toolconfig"
)

type console struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	token   string
	tokenAt time.Time
	env     []string
	home    string
	done    chan struct{}
	title   string
}

var consoleHTTP = &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{Proxy: nil, IdleConnTimeout: 10 * time.Second}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func (m *Manager) startConsole() error {
	home := filepath.Join(m.homePath(), "console-home")
	codexHome := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexHome, 0700); err != nil {
		return err
	}
	config := "model = " + tomlString(m.cfg.CodexModel) + "\nmodel_provider = \"workbench\"\n[model_providers.workbench]\nname = \"Workbench subscription\"\nbase_url = " + tomlString(strings.TrimRight(m.cfg.InferenceURL, "/")+"/codex") + "\nwire_api = \"responses\"\nenv_key = \"WORKBENCH_INFERENCE_GRANT\"\nsupports_websockets = false\n"
	config += toolconfig.CodexTOML()
	configPath := filepath.Join(codexHome, "config.toml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err = os.WriteFile(configPath, []byte(config), 0600); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if err := toolconfig.SyncCodex(configPath); err != nil {
		return err
	}
	base := filepath.Join(m.homePath(), "t3")
	if err := os.MkdirAll(filepath.Join(base, "userdata"), 0700); err != nil {
		return err
	}
	// This supervisor owns the only T3 process in a freshly started container.
	// PID numbers from a previous PID namespace can name a new Bun worker thread;
	// retain identity/history but discard the stale process-discovery record.
	if err := os.Remove(filepath.Join(base, "userdata/server-runtime.json")); err != nil && !os.IsNotExist(err) {
		return err
	}
	settingsPath := filepath.Join(base, "userdata/settings.json")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		selection := map[string]any{"instanceId": "codex", "model": m.cfg.CodexModel, "options": []any{}}
		settings := map[string]any{"enableProviderUpdateChecks": false, "enableAgentBrowserAccess": false, "defaultRuntimeMode": "full-access", "defaultModelSelection": selection, "textGenerationModelSelection": selection, "branchNamingMode": "static", "providerInstances": map[string]any{"codex": map[string]any{"driver": "codex", "enabled": true, "displayName": "Codex · Workbench subscription", "config": map[string]any{"binaryPath": "/usr/local/bin/codex-native", "homePath": codexHome, "setupMode": "existing"}}, "claudeAgent": map[string]any{"driver": "claudeAgent", "enabled": false}, "opencode": map[string]any{"driver": "opencode", "enabled": false}}}
		b, _ := json.Marshal(settings)
		if err = os.WriteFile(settingsPath, b, 0600); err != nil {
			return err
		}
	}

	// Migrate the initial prototype's old Claude driver name without overwriting
	// an explicit later choice in the native T3 settings.
	if b, err := os.ReadFile(settingsPath); err == nil {
		var settings map[string]any
		if json.Unmarshal(b, &settings) == nil {
			instances, _ := settings["providerInstances"].(map[string]any)
			if instances != nil {
				if _, ok := instances["claudeAgent"]; !ok {
					instances["claudeAgent"] = map[string]any{"driver": "claudeAgent", "enabled": false}
					delete(instances, "claude")
					encoded, _ := json.Marshal(settings)
					_ = os.WriteFile(settingsPath, encoded, 0600)
				}
			}
		}
	}
	env := []string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=" + home, "LANG=C.UTF-8", "TMPDIR=/tmp", "T3CODE_HOME=" + base, "CODEX_HOME=" + codexHome, "OPENAI_API_KEY=" + m.cfg.Token, "WORKBENCH_INFERENCE_GRANT=" + m.cfg.Token, "T3CODE_TELEMETRY_ENABLED=false", "T3CODE_OTEL_SDK_DISABLED=true", "DISABLE_AUTOUPDATER=1", "DISABLE_UPDATES=1", "DATA_DIR=" + m.cfg.DataDir}
	if m.cfg.EgressURL != "" && m.cfg.ConnectionMode != "open" {
		env = egress.Environment(env)
	}
	dir := filepath.Join(m.cfg.DataDir, "files")
	if _, err := os.Stat(filepath.Join(dir, ".git")); os.IsNotExist(err) {
		cmd := exec.Command("git", "init", "-b", "main", dir)
		cmd.Env = env
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		if err = cmd.Run(); err != nil {
			return err
		}
	}
	if m.cfg.Branch != "" {
		cmd := exec.Command("git", "-C", dir, "checkout", "-B", m.cfg.Branch)
		cmd.Env = env
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	cmd := exec.Command("/opt/t3/t3", "--mode", "web", "--host", "127.0.0.1", "--port", "3773", "--base-dir", base, "--no-browser", "--auto-bootstrap-project-from-cwd", dir)
	cmd.Env = env
	cmd.Dir = dir
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	m.console = &console{cmd: cmd, env: env, home: base, done: make(chan struct{}), title: m.cfg.Name}
	go func() { _ = cmd.Wait(); close(m.console.done) }()
	return nil
}

func (c *console) session(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	select {
	case <-c.done:
		return "", os.ErrProcessDone
	default:
	}
	if c.token != "" && time.Since(c.tokenAt) < 30*time.Minute {
		return c.token, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// Native auth issuance opens the same state store as server initialization.
	// Wait for startup/migrations to finish before launching the auth CLI.
	req, err := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:3773/.well-known/t3/environment", nil)
	if err != nil {
		return "", err
	}
	ready, err := consoleHTTP.Do(req)
	if err != nil {
		return "", err
	}
	ready.Body.Close()
	if ready.StatusCode != http.StatusOK {
		return "", os.ErrNotExist
	}
	cmd := exec.CommandContext(ctx, "/opt/t3/t3", "auth", "session", "issue", "--base-dir", c.home, "--ttl", "1h", "--token-only")
	cmd.Env = c.env
	out := &nativeOutput{}
	cmd.Stdout = out
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(out.Data))
	if value == "" || strings.ContainsAny(value, " \r\n\t") {
		return "", os.ErrInvalid
	}
	c.token = value
	c.tokenAt = time.Now()
	return value, nil
}

// Mutate the live server directly. The native project CLI can fall back to
// offline ownership and stop a running server when discovery is unavailable.
func (c *console) initializeTitle(ctx context.Context, origin, token, projectID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	marker := filepath.Join(c.home, ".initial-project-title")
	if _, err := os.Stat(marker); !os.IsNotExist(err) || c.title == "" {
		return
	}
	body, _ := json.Marshal(map[string]string{"type": "project.update", "commandId": shared.NewID("cmd"), "projectId": projectID, "title": c.title})
	req, err := http.NewRequestWithContext(ctx, "POST", origin+"/api/projects/mutate", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-t3-orchestration-protocol", "2")
	res, err := consoleHTTP.Do(req)
	if err != nil {
		return
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		_ = os.WriteFile(marker, []byte("set"), 0600)
	}
}

func (m *Manager) consoleProxy(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	quiescing := m.quiescing || m.closed
	m.mu.Unlock()
	if m.console == nil || quiescing {
		shared.Error(w, 503, "CONSOLE_UNAVAILABLE")
		return
	}
	token, err := m.console.session(r.Context())
	if err != nil {
		shared.Error(w, 503, "CONSOLE_NOT_READY")
		return
	}
	r.Header.Del("Cookie")
	shared.Proxy(w, r, "http://127.0.0.1:3773", "/"+r.PathValue("path"), token)
}

func (m *Manager) desktopProxy(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	unavailable := m.console == nil || m.quiescing || m.closed
	m.mu.Unlock()
	if unavailable {
		shared.Error(w, 503, "CONSOLE_UNAVAILABLE")
		return
	}
	// Preserve native client authority; never substitute the supervisor session.
	authorization := r.Header.Get(shared.NativeAuthorizationHeader)
	r.Header.Del(shared.NativeAuthorizationHeader)
	r.Header.Del("Cookie")
	if authorization != "" && !strings.HasPrefix(authorization, "Bearer ") && !strings.HasPrefix(authorization, "DPoP ") {
		shared.Error(w, 401, "UNAUTHORIZED")
		return
	}
	shared.ProxyNative(w, r, "http://127.0.0.1:3773", "/"+r.PathValue("path"), authorization)
}

func (m *Manager) consoleActivity() shared.Activity {
	a := shared.Activity{Known: true}
	if m.console == nil {
		return a
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	token, err := m.console.session(ctx)
	if err != nil {
		a.Known = false
		return a
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:3773/api/orchestration/shell", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("x-t3-orchestration-protocol", "2")
	res, err := consoleHTTP.Do(req)
	if err != nil {
		a.Known = false
		return a
	}
	defer res.Body.Close()
	var snapshot struct {
		Projects []struct {
			ID            string `json:"id"`
			WorkspaceRoot string `json:"workspaceRoot"`
		} `json:"projects"`
		Threads []struct {
			ID                string            `json:"id"`
			Status            string            `json:"status"`
			ActiveRunID       *string           `json:"activeRunId"`
			PendingBackground []json.RawMessage `json:"pendingBackgroundTasks"`
			UpdatedAt         string            `json:"updatedAt"`
		} `json:"threads"`
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, (8<<20)+1))
	if err != nil || len(b) > 8<<20 || res.StatusCode != 200 || json.Unmarshal(b, &snapshot) != nil || snapshot.Threads == nil {
		a.Known = false
		return a
	}
	for _, p := range snapshot.Projects {
		if p.WorkspaceRoot == filepath.Join(m.cfg.DataDir, "files") {
			m.console.initializeTitle(ctx, "http://127.0.0.1:3773", token, p.ID)
			break
		}
	}
	for _, t := range snapshot.Threads {
		state := "idle"
		if t.ActiveRunID != nil {
			state = "running"
		} else {
			switch t.Status {
			case "queued", "running", "waiting", "failed":
				state = t.Status
			case "preparing", "starting":
				state = "starting"
			}
		}
		m.eventMu.Lock()
		if m.observedT3 == nil {
			m.observedT3 = map[string]string{}
		}
		changed := m.observedT3[t.ID] != state && (len(m.observedT3) < 1024 || m.observedT3[t.ID] != "")
		if changed {
			m.observedT3[t.ID] = state
		}
		m.eventMu.Unlock()
		if changed {
			m.event("t3", "agent.state", "", "", state)
		}
		if t.ActiveRunID != nil || t.Status == "queued" || t.Status == "preparing" || t.Status == "starting" || t.Status == "running" || t.Status == "waiting" {
			a.ActiveRuns++
		}
		a.BackgroundJobs += len(t.PendingBackground)
		if updated, err := time.Parse(time.RFC3339Nano, t.UpdatedAt); err == nil && updated.After(a.LastActivity) {
			a.LastActivity = updated
		}
	}
	return a
}

// Only the registered T3 process and approved persistent protocol helpers are
// infrastructure. Terminals, command descendants and unknown processes count.
func (m *Manager) consoleInfrastructure(pid int, stat string) bool {
	if m.console == nil {
		return false
	}
	if pid == m.console.cmd.Process.Pid {
		return true
	}
	fields := strings.Fields(stat[strings.LastIndex(stat, ") ")+2:])
	if len(fields) < 2 {
		return false
	}
	ppid, _ := strconv.Atoi(fields[1])
	exe, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/exe")
	if err != nil {
		return false
	}
	if ppid == m.console.cmd.Process.Pid && exe == "/usr/local/bin/codex-native" {
		args, _ := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
		parts := strings.Split(string(args), "\x00")
		return len(parts) > 1 && parts[1] == "app-server"
	}
	if ppid == m.console.cmd.Process.Pid && strings.HasPrefix(exe, "/opt/t3/resource-monitor/") {
		return true
	}
	codeHost, _ := filepath.EvalSymlinks("/usr/local/bin/codex-code-mode-host")
	if exe == "/usr/local/bin/workspace-mcp" || (codeHost != "" && exe == codeHost) {
		parentExe, _ := os.Readlink("/proc/" + strconv.Itoa(ppid) + "/exe")
		return parentExe == "/usr/local/bin/codex-native"
	}
	return false
}

func tomlString(s string) string { b, _ := json.Marshal(s); return string(b) }
