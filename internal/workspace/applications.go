package workspace

import (
	"agent-workbench/internal/shared"
	"agent-workbench/internal/workspace/setup"
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type applicationProcess struct {
	rootPID   int
	rootStart string
	cmd       *exec.Cmd
	log       *applicationLog
}
type applicationLog struct {
	sync.Mutex
	content []byte
}

func (l *applicationLog) Write(b []byte) (int, error) {
	l.Lock()
	defer l.Unlock()
	l.content = append(l.content, b...)
	if len(l.content) > 65536 {
		l.content = append([]byte(nil), l.content[len(l.content)-65536:]...)
	}
	return len(b), nil
}
func (l *applicationLog) text() string { l.Lock(); defer l.Unlock(); return string(l.content) }

type applicationView struct {
	retainedService
	Application shared.ApplicationConfig `json:"application"`
	Ready       bool                     `json:"ready"`
	Logs        string                   `json:"logs,omitempty"`
}

func (m *Manager) applicationView(id string) (applicationView, error) {
	var v applicationView
	var body string
	s, err := m.service(id)
	if err != nil {
		return v, err
	}
	v.retainedService = s
	if err = m.db.QueryRow(`SELECT configuration FROM workspace_applications WHERE service_id=?`, id).Scan(&body); err != nil {
		return v, err
	}
	if err = json.Unmarshal([]byte(body), &v.Application); err != nil {
		return v, err
	}
	if s.State == "active" && v.Application.Kind == "forward" {
		_, v.Ready = forwardTarget(v.Application)
		if !v.Ready {
			m.db.Exec(`UPDATE retained_services SET state='stopped' WHERE id=?`, id)
			v.State = "stopped"
		}
	} else if s.State == "active" {
		c, e := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(v.Application.Port)), 200*time.Millisecond)
		if e == nil {
			v.Ready = true
			c.Close()
		}
	}
	if process := m.applications[id]; process != nil {
		v.Logs = process.log.text()
	}
	return v, nil
}
func (m *Manager) listApplications(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rows, err := m.db.Query(`SELECT service_id FROM workspace_applications ORDER BY rowid DESC`)
	if err != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	items := []applicationView{}
	for _, id := range ids {
		if v, e := m.applicationView(id); e == nil {
			items = append(items, v)
		}
	}
	shared.JSON(w, 200, map[string]any{"items": items})
}
func (m *Manager) createApplication(w http.ResponseWriter, r *http.Request, a shared.ApplicationConfig, key string, ttl int) {
	if !a.Valid() || !shared.ValidKey(key) || ttl < 1 || ttl > 86400 {
		shared.Error(w, 400, "INVALID_APPLICATION")
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.quiescing {
		shared.Error(w, 409, "WORKSPACE_STOPPING")
		return
	}
	body, _ := json.Marshal(a)
	fingerprint := fmt.Sprintf("%x:%d", sha256.Sum256(body), ttl)
	var prior, id string
	err := m.db.QueryRow(`SELECT fingerprint,service_id FROM workspace_applications WHERE idem_key=?`, key).Scan(&prior, &id)
	if err == nil {
		if prior != fingerprint {
			shared.Error(w, 409, "IDEMPOTENCY_CONFLICT")
			return
		}
		v, e := m.applicationView(id)
		if e != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, v)
		return
	}
	if err != sql.ErrNoRows {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	count := 0
	m.db.QueryRow(`SELECT count(*) FROM retained_services WHERE state='active' AND expires_at>?`, time.Now().Unix()).Scan(&count)
	if count >= 4 {
		shared.Error(w, 409, "APPLICATION_LIMIT")
		return
	}
	if a.Kind == "desktop" {
		var desktops int
		if err := m.db.QueryRow(`SELECT count(*) FROM workspace_applications a JOIN retained_services s ON s.id=a.service_id WHERE s.state='active' AND s.expires_at>? AND json_extract(a.configuration,'$.kind')='desktop'`, time.Now().Unix()).Scan(&desktops); err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		if desktops > 0 {
			shared.Error(w, 409, "DESKTOP_ALREADY_RUNNING")
			return
		}
		if _, err := exec.LookPath("workbench-desktop"); err != nil {
			shared.Error(w, 503, "DESKTOP_NOT_INSTALLED")
			return
		}
	}
	if a.Kind == "forward" {
		m.createForwardLocked(w, r, a, key, ttl, fingerprint)
		return
	}
	// Refuse existing listeners, including workspace administration services.
	listener, e := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(a.Port)))
	if e != nil {
		shared.Error(w, 409, "APPLICATION_PORT_BUSY")
		return
	}
	listener.Close()
	dir, e := m.root.Open(a.Directory)
	if e != nil {
		shared.Error(w, 400, "APPLICATION_DIRECTORY_UNAVAILABLE")
		return
	}
	defer dir.Close()
	info, e := dir.Stat()
	if e != nil || !info.IsDir() {
		shared.Error(w, 400, "APPLICATION_DIRECTORY_UNAVAILABLE")
		return
	}
	id = shared.NewID("service_")
	cmd := exec.Command("/bin/sh", "-c", a.Command)
	cmd.Dir = fmt.Sprintf("/proc/%d/fd/%d", os.Getpid(), dir.Fd())
	cmd.Env = []string{"PATH=" + setup.Path(m.homePath()), "HOME=" + m.homePath(), "PORT=" + strconv.Itoa(a.Port), "HOST=127.0.0.1", "PYTHONUNBUFFERED=1", "LANG=C.UTF-8"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	log := &applicationLog{}
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.WaitDelay = time.Second
	tx, e := m.db.Begin()
	if e != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	defer tx.Rollback()
	_, e = tx.Exec(`INSERT INTO retained_services VALUES(?,'active',?,'{}')`, id, time.Now().Unix()+int64(ttl))
	if e == nil {
		_, e = tx.Exec(`INSERT INTO workspace_applications(service_id,idem_key,fingerprint,configuration) VALUES(?,?,?,?)`, id, key, fingerprint, string(body))
	}
	if e != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	if e = cmd.Start(); e != nil {
		shared.Error(w, 503, "APPLICATION_START_FAILED")
		return
	}
	if e = tx.Commit(); e != nil {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		cmd.Wait()
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	if m.applications == nil {
		m.applications = map[string]*applicationProcess{}
	}
	m.applications[id] = &applicationProcess{cmd: cmd, log: log}
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		cmd.Wait()
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		m.mu.Lock()
		defer m.mu.Unlock()
		m.applications[id].cmd = nil
		m.db.Exec(`UPDATE retained_services SET state='stopped' WHERE id=? AND state='active'`, id)
	}()
	v, _ := m.applicationView(id)
	shared.JSON(w, 201, v)
}
func (m *Manager) stopApplication(id string) {
	if p := m.applications[id]; p != nil {
		p.rootPID = 0
		p.rootStart = ""
	}
	if p := m.applications[id]; p != nil && p.cmd != nil {
		syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
	}
}
func (m *Manager) reapApplications() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id := range m.applications {
		if s, e := m.service(id); e == nil && s.State != "active" {
			m.stopApplication(id)
		}
	}
}

type applicationFetch struct {
	Path        string `json:"path"`
	Method      string `json:"method,omitempty"`
	Query       string `json:"query,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Body        []byte `json:"body,omitempty"`
}

func (m *Manager) fetchApplication(w http.ResponseWriter, r *http.Request, s retainedService, in applicationFetch, reply func(map[string]any)) bool {
	var raw string
	err := m.db.QueryRow(`SELECT configuration FROM workspace_applications WHERE service_id=?`, s.ID).Scan(&raw)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return true
	}
	var a shared.ApplicationConfig
	if json.Unmarshal([]byte(raw), &a) != nil || !a.Valid() {
		shared.Error(w, 503, "INVALID_APPLICATION")
		return true
	}
	if in.Method == "" {
		in.Method = "GET"
	}
	switch in.Method {
	case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS":
	default:
		shared.Error(w, 400, "INVALID_REQUEST")
		return true
	}
	if len(in.Body) > 1<<20 || len(in.Query) > 8192 || strings.ContainsAny(in.ContentType, "\r\n") {
		shared.Error(w, 400, "INVALID_REQUEST")
		return true
	}
	host := net.JoinHostPort("127.0.0.1", strconv.Itoa(a.Port))
	if a.Kind == "forward" {
		p, ok := forwardTarget(a)
		if !ok {
			shared.Error(w, 410, "APPLICATION_PORT_CLOSED")
			return true
		}
		host = p.Address
	}
	target := url.URL{Scheme: "http", Host: host, Path: "/" + in.Path, RawQuery: in.Query}
	req, err := http.NewRequestWithContext(r.Context(), in.Method, target.String(), bytes.NewReader(in.Body))
	if err != nil {
		shared.Error(w, 400, "INVALID_REQUEST")
		return true
	}
	req.Header.Set("Content-Type", in.ContentType)
	client := &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		shared.Error(w, 503, "APPLICATION_NOT_READY")
		return true
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, (2<<20)+1))
	if err != nil || len(data) > 2<<20 {
		shared.Error(w, 502, "APPLICATION_RESPONSE_TOO_LARGE")
		return true
	}
	location := res.Header.Get("Location")
	if location != "" {
		u, e := url.Parse(location)
		if e != nil || u.IsAbs() || u.Host != "" || strings.HasPrefix(location, "//") {
			location = ""
		}
	}
	contentType := res.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	reply(map[string]any{"content": data, "content_type": contentType, "status": res.StatusCode, "location": location, "websocket": false})
	return true
}

// Match kernel process ancestry, never a process name supplied by the app.
// Interactive shells can create new sessions, so follow parents as well as pgrp.
func (m *Manager) ownedApplicationProcess(pid int, stat string) bool {
	roots := map[int]bool{}
	for _, app := range m.applications {
		if app.cmd != nil {
			roots[app.cmd.Process.Pid] = true
		}
		if app.rootPID > 0 && app.rootStart != "" && processStart(app.rootPID) == app.rootStart {
			roots[app.rootPID] = true
		}
	}
	if len(roots) == 0 {
		return false
	}
	for depth := 0; depth < 64 && pid > 1; depth++ {
		if roots[pid] {
			return true
		}
		end := strings.LastIndex(stat, ") ")
		if end < 0 {
			return false
		}
		fields := strings.Fields(stat[end+2:])
		if len(fields) < 3 {
			return false
		}
		group, _ := strconv.Atoi(fields[2])
		if roots[group] {
			return true
		}
		parent, err := strconv.Atoi(fields[1])
		if err != nil || parent == pid {
			return false
		}
		pid = parent
		body, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if err != nil {
			return false
		}
		stat = string(body)
	}
	return false
}

func (m *Manager) createForwardLocked(w http.ResponseWriter, r *http.Request, a shared.ApplicationConfig, key string, ttl int, fingerprint string) {
	port, ok := forwardTarget(a)
	if !ok || !probeHTTP(r.Context(), port) {
		shared.Error(w, 409, "APPLICATION_PORT_UNAVAILABLE")
		return
	}
	id := shared.NewID("service_")
	body, _ := json.Marshal(a)
	tx, err := m.db.Begin()
	if err != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT INTO retained_services VALUES(?,'active',?,'{}')`, id, time.Now().Unix()+int64(ttl))
	if err == nil {
		_, err = tx.Exec(`INSERT INTO workspace_applications VALUES(?,?,?,?)`, id, key, fingerprint, string(body))
	}
	if err != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	if err = tx.Commit(); err != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	pid, start := listenerOwner(port.Inode)
	if m.applications == nil {
		m.applications = map[string]*applicationProcess{}
	}
	m.applications[id] = &applicationProcess{rootPID: pid, rootStart: start, log: &applicationLog{}}
	view, _ := m.applicationView(id)
	shared.JSON(w, 201, view)
}
