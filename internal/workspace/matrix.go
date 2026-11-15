package workspace

import (
	"agent-workbench/internal/egress"
	"agent-workbench/internal/shared"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

type matrixProcess struct {
	cmd  *exec.Cmd
	done chan struct{}
	home string
}

func (m *Manager) startMatrix(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cfg.AgentKind != "hermes" || m.cfg.HermesMatrix == nil {
		shared.Error(w, 409, "MATRIX_RESTART_REQUIRED")
		return
	}
	if m.matrix != nil {
		select {
		case <-m.matrix.done:
		default:
			shared.JSON(w, 202, map[string]string{"state": "starting"})
			return
		}
	}
	if m.closed || m.quiescing {
		shared.Error(w, 409, "WORKSPACE_BUSY")
		return
	}
	home := filepath.Join(m.homePath(), "agents", "hermes")
	if os.MkdirAll(home, 0700) != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	_ = os.Remove(filepath.Join(home, "matrix-connected"))
	config, _ := json.Marshal(m.cfg.HermesMatrix)
	cmd := exec.Command("/usr/local/bin/hermes-matrix")
	cmd.Dir = filepath.Join(m.cfg.DataDir, "files")
	cmd.Env = []string{"PATH=/opt/hermes/.venv/bin:/usr/local/bin:/usr/bin:/bin", "HOME=" + home, "HERMES_HOME=" + home, "HERMES_MODEL=" + m.cfg.HermesModel, "INFERENCE_URL=" + m.cfg.InferenceURL, "WORKBENCH_INFERENCE_GRANT=" + m.cfg.Token, "HERMES_MATRIX_CONFIG=" + string(config), "PYTHONPATH=/opt/hermes", "TERMINAL_ENV=local", "LANG=C.UTF-8"}

	credential := filepath.Join(home, "workbench-mcp-token")
	if os.WriteFile(credential, []byte(m.cfg.Token), 0600) != nil {
		shared.Error(w, 503, "MCP_CREDENTIAL_UNAVAILABLE")
		return
	}
	cmd.Env = append(cmd.Env, "WORKBENCH_MCP_URL=http://127.0.0.1:18085/mcp", "WORKBENCH_MCP_TOKEN_FILE="+credential, "WORKBENCH_PROJECT_ID="+m.cfg.ProjectID)
	if m.cfg.EgressURL != "" && m.cfg.ConnectionMode != "open" {
		cmd.Env = egress.Environment(cmd.Env, m.cfg.InferenceURL, m.cfg.BrokerURL)
	}
	events := &nativeEvents{m: m, source: "matrix"}
	cmd.Stdout = events
	cmd.Stderr = &nativeEvents{m: m, source: "matrix"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if cmd.Start() != nil {
		m.event("matrix", "service.failed", "", "", "")
		shared.Error(w, 503, "MATRIX_UNAVAILABLE")
		return
	}
	p := &matrixProcess{cmd: cmd, done: make(chan struct{}), home: home}
	m.matrix = p
	m.event("matrix", "service.started", "", "", "")
	go func() {
		err := cmd.Wait()
		kind := "service.stopped"
		if err != nil {
			kind = "service.failed"
		}
		m.event("matrix", kind, "", "", "")
		_ = os.Remove(filepath.Join(home, "matrix-connected"))
		close(p.done)
	}()
	shared.JSON(w, 202, map[string]string{"state": "starting"})
}
func (m *Manager) stopMatrix() {
	m.mu.Lock()
	p := m.matrix
	m.mu.Unlock()
	if p == nil {
		return
	}
	defer func() {
		m.mu.Lock()
		if m.matrix == p {
			m.matrix = nil
		}
		m.mu.Unlock()
	}()
	select {
	case <-p.done:
		return
	default:
	}
	_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGTERM)
	select {
	case <-p.done:
	case <-time.After(5 * time.Second):
		_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
		<-p.done
	}
}
func (m *Manager) matrixStatus(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	p := m.matrix
	m.mu.Unlock()
	state := "disconnected"
	if p != nil {
		select {
		case <-p.done:
			state = "failed"
		default:
			state = "starting"
			if _, err := os.Stat(filepath.Join(p.home, "matrix-connected")); err == nil {
				state = "connected"
			}
		}
	}
	shared.JSON(w, 200, map[string]string{"state": state})
}
