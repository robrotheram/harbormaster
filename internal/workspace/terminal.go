package workspace

import (
	"agent-workbench/internal/workspace/setup"
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
	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

const terminalBufferLimit = 256 << 10
const terminalLease = 60 * time.Second

type terminalSession struct {
	mu       sync.Mutex
	ioMu     sync.Mutex
	cmd      *exec.Cmd
	tty      *os.File
	output   []byte
	base     int64
	exited   bool
	lastSeen time.Time
	done     chan struct{}
	home     string
	stopOnce sync.Once
}

type terminalInput struct {
	ID     string `json:"id"`
	Input  string `json:"input"`
	Offset int64  `json:"offset"`
	Cols   uint16 `json:"cols"`
	Rows   uint16 `json:"rows"`
}

func terminalSize(cols, rows uint16) bool {
	return cols >= 2 && cols <= 500 && rows >= 1 && rows <= 200
}

func (m *Manager) createTerminal(w http.ResponseWriter, r *http.Request) {
	var in terminalInput
	if !shared.Decode(w, r, &in, 1024) {
		return
	}
	if !shared.ValidID(in.ID) || !terminalSize(in.Cols, in.Rows) || in.Input != "" || in.Offset != 0 {
		shared.Error(w, 400, "INVALID_REQUEST")
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.quiescing {
		shared.Error(w, 409, "WORKSPACE_STOPPING")
		return
	}
	if m.terminals == nil {
		m.terminals = map[string]*terminalSession{}
	}
	if _, exists := m.terminals[in.ID]; exists {
		shared.JSON(w, 200, map[string]string{"id": in.ID})
		return
	}
	if len(m.terminals) >= 4 {
		shared.Error(w, 429, "TERMINAL_LIMIT")
		return
	}
	shell := "/bin/bash"
	args := []string{"--noprofile", "--norc", "-i"}
	if _, err := os.Stat(shell); err != nil {
		shell, args = "/bin/sh", []string{"-i"}
	}
	home, err := os.MkdirTemp("", "workbench-terminal-")
	if err != nil {
		shared.Error(w, 503, "TERMINAL_UNAVAILABLE")
		return
	}
	cmd := exec.Command(shell, args...)
	cmd.Dir = filepath.Join(m.cfg.DataDir, "files")
	// Do not inherit management credentials from the workspace service process.
	cmd.Env = []string{"PATH=" + setup.Path(m.homePath()), "HOME=" + home, "TERM=xterm-256color", "LANG=C.UTF-8", "SHELL=" + shell}
	if m.cfg.EgressURL != "" {
		cmd.Env = egress.Environment(cmd.Env, m.cfg.InferenceURL, m.cfg.BrokerURL)
	}
	tty, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: in.Cols, Rows: in.Rows})
	if err != nil {
		_ = os.Remove(home)
		shared.Error(w, 503, "TERMINAL_UNAVAILABLE")
		return
	}
	// Enable deadlines and interruptible reads on the PTY.
	if err = syscall.SetNonblock(int(tty.Fd()), true); err != nil {
		_ = cmd.Process.Kill()
		_ = tty.Close()
		_ = cmd.Wait()
		_ = os.Remove(home)
		shared.Error(w, 503, "TERMINAL_UNAVAILABLE")
		return
	}
	session := &terminalSession{cmd: cmd, tty: tty, lastSeen: time.Now(), done: make(chan struct{}), home: home}
	m.terminals[in.ID] = session
	m.lastActivity = time.Now().UTC()
	m.wg.Add(1)
	go m.runTerminal(in.ID, session)
	shared.JSON(w, 201, map[string]string{"id": in.ID})
}

func (m *Manager) runTerminal(id string, s *terminalSession) {
	defer m.wg.Done()
	defer os.RemoveAll(s.home)
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buffer := make([]byte, 8192)
		for {
			n, err := s.tty.Read(buffer)
			if n > 0 {
				s.mu.Lock()
				s.output = append(s.output, buffer[:n]...)
				if excess := len(s.output) - terminalBufferLimit; excess > 0 {
					s.base += int64(excess)
					s.output = append([]byte(nil), s.output[excess:]...)
				}
				s.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	exited := make(chan struct{})
	go func() { _ = s.cmd.Wait(); close(exited) }()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-exited:
			s.ioMu.Lock()
			// Drain final output, including an incomplete UTF-8 sequence as raw bytes.
			select {
			case <-readDone:
			case <-time.After(100 * time.Millisecond):
			}
			s.stop()
			<-readDone
			s.mu.Lock()
			s.exited = true
			s.mu.Unlock()
			close(s.done)
			s.ioMu.Unlock()
			// Keep the exit output available until the client closes or its lease expires.
			exited = nil
		case <-m.historyStop:
			s.stop()
			if exited != nil {
				<-exited
				<-readDone
			}
			return
		case <-ticker.C:
			s.mu.Lock()
			expired := time.Since(s.lastSeen) > terminalLease
			s.mu.Unlock()
			if expired {
				s.stop()
			}
			m.mu.Lock()
			_, present := m.terminals[id]
			if expired {
				delete(m.terminals, id)
			}
			closing := m.closed
			m.mu.Unlock()
			if !present || expired || closing {
				s.stop()
				if exited != nil {
					<-exited
					<-readDone
					s.mu.Lock()
					s.exited = true
					s.mu.Unlock()
					close(s.done)
				}
				return
			}
		}
	}
}

// A job-control shell creates separate process groups for foreground commands.
// Kill all members of its session before the shell, rather than just its pgrp.
func (s *terminalSession) stop() {
	s.stopOnce.Do(func() {
		entries, _ := os.ReadDir("/proc")
		for _, entry := range entries {
			pid, err := strconv.Atoi(entry.Name())
			if err != nil || pid == s.cmd.Process.Pid {
				continue
			}
			stat, err := os.ReadFile("/proc/" + entry.Name() + "/stat")
			if err != nil {
				continue
			}
			end := strings.LastIndex(string(stat), ") ")
			if end < 0 {
				continue
			}
			fields := strings.Fields(string(stat)[end+2:])
			if len(fields) > 3 && fields[3] == strconv.Itoa(s.cmd.Process.Pid) {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
		}
		_ = s.cmd.Process.Kill()
		_ = s.tty.Close()
	})
}

func (m *Manager) terminalRoute(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("terminal")
	if !shared.ValidID(id) {
		shared.Error(w, 400, "INVALID_REQUEST")
		return
	}
	var in terminalInput
	if !shared.Decode(w, r, &in, 32<<10) {
		return
	}
	if in.ID != "" || in.Offset < 0 || len(in.Input) > 4096 || (r.PathValue("action") == "exchange" && !terminalSize(in.Cols, in.Rows)) {
		shared.Error(w, 400, "INVALID_REQUEST")
		return
	}
	m.mu.Lock()
	s := m.terminals[id]
	if r.PathValue("action") == "close" {
		delete(m.terminals, id)
	}
	m.lastActivity = time.Now().UTC()
	m.mu.Unlock()
	if r.PathValue("action") == "close" {
		if s != nil {
			s.stop()
		}
		shared.JSON(w, 200, map[string]bool{"closed": true})
		return
	}
	if r.PathValue("action") != "exchange" {
		shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
		return
	}
	if s == nil {
		shared.Error(w, 404, "TERMINAL_EXPIRED")
		return
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	s.mu.Lock()
	s.lastSeen = time.Now()
	exited := s.exited
	end := s.base + int64(len(s.output))
	s.mu.Unlock()
	if in.Offset > end {
		shared.Error(w, 400, "INVALID_REQUEST")
		return
	}
	if !exited {
		if err := resizeTerminal(s.tty, in.Cols, in.Rows); err != nil {
			shared.Error(w, 409, "TERMINAL_EXPIRED")
			return
		}
		if in.Input != "" {
			_ = s.tty.SetWriteDeadline(time.Now().Add(time.Second))
			if _, err := s.tty.Write([]byte(in.Input)); err != nil {
				shared.Error(w, 409, "TERMINAL_EXPIRED")
				return
			}
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	offset := in.Offset
	truncated := offset < s.base
	if truncated {
		offset = s.base
	}
	data := append([]byte{}, s.output[offset-s.base:]...)
	shared.JSON(w, 200, map[string]any{"output": data, "offset": s.base + int64(len(s.output)), "exited": s.exited, "truncated": truncated})
}

// SyscallConn holds the descriptor alive during ioctl and preserves nonblocking
// mode, unlike File.Fd(), which also races with concurrent terminal shutdown.
func resizeTerminal(tty *os.File, cols, rows uint16) error {
	raw, err := tty.SyscallConn()
	if err != nil {
		return err
	}
	var ioctlErr error
	err = raw.Control(func(fd uintptr) {
		ioctlErr = unix.IoctlSetWinsize(int(fd), unix.TIOCSWINSZ, &unix.Winsize{Col: cols, Row: rows})
	})
	if err != nil {
		return err
	}
	return ioctlErr
}
