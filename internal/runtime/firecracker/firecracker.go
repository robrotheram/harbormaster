package firecracker

// Firecracker runs on the host, never inside an agent container. Guests have
// no NIC, no host filesystem mounts, and no arbitrary host socket forwarding.
import (
	"agent-workbench/internal/runtime/protocol"
	"agent-workbench/internal/shared"
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/maphash"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const fcBase = "/opt/agent-workbench/firecracker"
const fcState = "/var/lib/agent-workbench/firecracker"
const fcJails = "/srv/agent-workbench-jailer"
const fcCgroups = "/sys/fs/cgroup/awb-firecracker"

type fcVM struct {
	ID, Generation, Jail, NetNS string
	UID                         int
	Spec                        protocol.WorkspaceSpec
	Policy                      shared.ConnectionPolicy
	listeners                   []net.Listener
	slots                       chan struct{}
	running                     bool
	stopping                    bool
}
type Firecracker struct {
	token, service, secret string
	mu                     sync.Mutex
	lifecycle              sync.RWMutex
	allocation             sync.Mutex
	workspaceOps           [64]sync.Mutex
	vms                    map[string]*fcVM
	slots                  chan struct{}
	artifacts              map[string]os.FileInfo
	hermes                 bool
	matrixOwners           map[string]string
	terminateCgroup        func(string) error
}

// Bound console storage without limiting block-device writes to low offsets.
// RLIMIT_FSIZE applies to data disks too, so it cannot serve as a log limit.
type fcConsole struct {
	mu        sync.Mutex
	file      io.Writer
	remaining int
}

func (c *fcConsole) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	size := len(p)
	if len(p) > c.remaining {
		p = p[:c.remaining]
	}
	if len(p) > 0 {
		n, err := c.file.Write(p)
		c.remaining -= n
		if err != nil {
			return n, err
		}
	}
	return size, nil
}

func trustedFile(path string) error {
	for p := path; p != "/"; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0022 != 0 {
			return fmt.Errorf("untrusted path %s", p)
		}
		if !rootOwned(info) {
			return fmt.Errorf("path is not root-owned: %s", p)
		}
	}
	return nil
}

// Check installed artifacts against a root-owned manifest, rather than accepting
// user-selected kernels, Firecracker options, images, or an 'isolation=true' flag.
func firecrackerPreflight() error {
	if os.Geteuid() != 0 {
		return errors.New("Firecracker host controller requires root")
	}
	if err := trustedFile(fcBase + "/manifest.json"); err != nil {
		return err
	}
	var manifest map[string]string
	b, err := os.ReadFile(fcBase + "/manifest.json")
	if err != nil {
		return err
	}
	if err = json.Unmarshal(b, &manifest); err != nil {
		return err
	}
	names := []string{"firecracker", "jailer", "vmlinux", "rootfs.ext4"}
	if manifest["hermes-runtime.json"] != "" {
		names = append(names, "hermes-runtime.json")
	}
	for _, name := range names {
		p := fcBase + "/" + name
		if err := trustedFile(p); err != nil {
			return err
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		h := sha256.New()
		_, err = io.Copy(h, f)
		f.Close()
		if err != nil || manifest[name] == "" || hex.EncodeToString(h.Sum(nil)) != manifest[name] {
			return fmt.Errorf("artifact integrity failure: %s", name)
		}
	}
	k, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
	if err != nil {
		return err
	}
	k.Close()
	for _, p := range []string{"/sys/fs/cgroup/cgroup.controllers", fcCgroups + "/cgroup.controllers"} {
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, c := range []string{"cpu", "memory", "pids"} {
			if !strings.Contains(" "+string(b)+" ", " "+c+" ") && !strings.Contains(string(b), c+"\n") {
				return fmt.Errorf("missing cgroup controller %s", c)
			}
		}
	}
	return nil
}

func NewFirecracker(token, service, secret string) (*Firecracker, error) {
	if len(token) < 32 || len(service) < 32 || len(secret) < 32 {
		return nil, errors.New("missing controller credentials")
	}
	if err := firecrackerPreflight(); err != nil {
		return nil, err
	}
	// On a daemon restart, fail closed by killing stale cgroups before accepting
	// requests. Never adopt a running VM based only on persisted metadata or PID.
	groups, err := os.ReadDir(fcCgroups)
	if err != nil {
		return nil, err
	}
	for _, g := range groups {
		if g.IsDir() && strings.HasPrefix(g.Name(), "fc-") {
			if err := os.WriteFile(filepath.Join(fcCgroups, g.Name(), "cgroup.kill"), []byte("1"), 0600); err != nil {
				return nil, err
			}
		}
	}
	artifacts := map[string]os.FileInfo{}
	var manifest map[string]string
	b, _ := os.ReadFile(fcBase + "/manifest.json")
	_ = json.Unmarshal(b, &manifest)
	hermes := manifest["hermes-runtime.json"] != ""
	names := []string{"manifest.json", "firecracker", "jailer", "vmlinux", "rootfs.ext4"}
	if hermes {
		names = append(names, "hermes-runtime.json")
	}
	for _, name := range names {
		p := fcBase + "/" + name
		i, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		artifacts[p] = i
	}
	return &Firecracker{token: token, service: service, secret: secret, vms: map[string]*fcVM{}, slots: make(chan struct{}, 128), artifacts: artifacts, hermes: hermes}, nil
}

func (f *Firecracker) checkArtifacts() error {
	expected := 5
	if f.hermes {
		expected++
	}
	if len(f.artifacts) != expected {
		return errors.New("artifact verification missing")
	}
	// Hash the complete read-only image at daemon admission. Before each VM,
	// fence replacement or mutation of those root-owned artifacts without
	// rehashing six GiB inside the workspace HTTP startup deadline.
	for p, prior := range f.artifacts {
		if err := trustedFile(p); err != nil {
			return err
		}
		now, err := os.Stat(p)
		if err != nil {
			return err
		}
		if !os.SameFile(prior, now) || prior.Size() != now.Size() || prior.Mode() != now.Mode() || !prior.ModTime().Equal(now.ModTime()) {
			return errors.New("installed artifacts changed; controller restart and hash verification required")
		}
	}
	return nil
}
func (f *Firecracker) grant(v *fcVM) string {
	return protocol.Grant(f.secret, v.ID, v.Generation)
}
func fcCommand(ctx context.Context, name string, args ...string) error {
	c := exec.CommandContext(ctx, name, args...)
	c.Env = []string{"PATH=/usr/bin:/bin", "LANG=C"}
	if b, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w: %.512s", name, err, b)
	}
	return nil
}
func fcConfig(memory int) map[string]any {
	return map[string]any{
		"boot-source":    map[string]any{"kernel_image_path": "/vmlinux", "boot_args": "console=ttyS0 reboot=k panic=1 pci=off root=/dev/vda ro init=/sbin/awb-init random.trust_cpu=on"},
		"drives":         []any{map[string]any{"drive_id": "root", "path_on_host": "/rootfs.ext4", "is_root_device": true, "is_read_only": true}, map[string]any{"drive_id": "data", "path_on_host": "/data.ext4", "is_root_device": false, "is_read_only": false}, map[string]any{"drive_id": "config", "path_on_host": "/config.ext4", "is_root_device": false, "is_read_only": true}},
		"machine-config": map[string]any{"vcpu_count": 1, "mem_size_mib": memory, "smt": false},
		"vsock":          map[string]any{"guest_cid": 3, "uds_path": "/v.sock"},
		// No network-interfaces, MMDS, balloon, host devices, or shared filesystems.
	}
}
func (f *Firecracker) launch(ctx context.Context, id string, spec protocol.WorkspaceSpec) (v *fcVM, err error) {
	if err := protocol.CheckResources(spec.Resources, protocol.ResourceLimits{CPUCores: 1, MemoryBytes: 1 << 30}); err != nil {
		return nil, err
	}
	isHermes := spec.AgentKind == "hermes"
	if isHermes {
		if !f.hermes || !projectMCPEnabled() || spec.HermesModel == "" || len(spec.HermesModel) > 128 || strings.ContainsAny(spec.HermesModel, " \r\n\t\x00") {
			return nil, errors.New("Hermes runtime or configured model unavailable")
		}
	}
	spec, err = protocol.NormalizeSpec(spec)
	if err != nil {
		return nil, err
	}
	if !shared.ValidID(id) || spec.ConnectionMode == "open" || spec.ParentID != "" {
		return nil, errors.New("unsupported sandbox spec (open networking and parent seeding are refused)")
	}
	f.allocation.Lock()
	allocatedSlot := true
	defer func() {
		if allocatedSlot {
			f.allocation.Unlock()
		}
	}()
	f.mu.Lock()
	old := f.vms[id]
	oldRunning := old != nil && old.running && !old.stopping
	count := 0
	for _, x := range f.vms {
		if x.running {
			count++
		}
	}
	f.mu.Unlock()
	if oldRunning {
		if checkFCSpec(old.Spec, spec) != nil {
			return nil, errors.New("workspace ownership mismatch")
		}
		return old, nil
	}
	if old != nil {
		if err := f.kill(old); err != nil {
			return nil, err
		}
	}
	if count >= 4 {
		return nil, errors.New("sandbox capacity exhausted")
	}
	if err = f.checkArtifacts(); err != nil {
		return nil, err
	}
	dir := filepath.Join(fcState, id)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	persisted := dir + "/owner.json"
	if b, e := os.ReadFile(persisted); e == nil {
		var prior protocol.WorkspaceSpec
		if json.Unmarshal(b, &prior) != nil || checkFCSpec(prior, spec) != nil {
			return nil, errors.New("workspace ownership mismatch")
		}
	} else if !os.IsNotExist(e) {
		return nil, e
	} else {
		b, _ := json.Marshal(spec)
		if err = os.WriteFile(persisted, b, 0600); err != nil {
			return nil, err
		}
	}
	generation := "fc-" + shared.NewID("")
	// Unique UID per generation, even after controller restarts; never share VMM UIDs.
	counterFile := fcState + "/next-uid"
	next := 200000
	if b, e := os.ReadFile(counterFile); e == nil {
		next, e = strconv.Atoi(strings.TrimSpace(string(b)))
		if e != nil || next < 200000 || next >= 400000 {
			return nil, errors.New("UID pool unavailable")
		}
	}
	if err = os.WriteFile(counterFile, []byte(strconv.Itoa(next+1)), 0600); err != nil {
		return nil, err
	}
	v = &fcVM{ID: id, Generation: generation, UID: next, Spec: spec, Policy: shared.ConnectionPolicy{Mode: "blocked", Destinations: []string{}}, Jail: fcJails + "/firecracker/" + generation + "/root", NetNS: "awb-" + generation}
	allocated := v
	defer func() {
		if err != nil {
			f.kill(allocated)
		}
	}()
	if err = os.MkdirAll(v.Jail, 0700); err != nil {
		return nil, err
	}
	for _, n := range []string{"vmlinux", "rootfs.ext4"} {
		if err = os.Link(fcBase+"/"+n, v.Jail+"/"+n); err != nil {
			return nil, err
		}
	}
	data := dir + "/data.ext4"
	if _, e := os.Stat(data); os.IsNotExist(e) {
		d, e := os.OpenFile(data, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
		if e != nil {
			return nil, e
		}
		e = d.Truncate(8 << 30)
		d.Close()
		if e != nil {
			return nil, e
		}
		if err = fcCommand(ctx, "mkfs.ext4", "-q", "-F", "-L", "awb-data", data); err != nil {
			return nil, err
		}
	} else if e != nil {
		return nil, e
	}
	// A private directory, pinned owner record and lifecycle lock fence writers.
	if err = os.Chown(data, next, next); err != nil {
		return nil, err
	}
	if err = os.Link(data, v.Jail+"/data.ext4"); err != nil {
		return nil, err
	}
	configDir := dir + "/boot-" + generation
	if err = os.Mkdir(configDir, 0700); err != nil {
		return nil, err
	}
	env := map[string]string{"WORKSPACE_ID": id, "PROJECT_ID": spec.ProjectID, "AGENT_HOME_ID": spec.HomeID, "AGENT_KIND": spec.AgentKind, "WORKSPACE_NAME": spec.Name, "WORKSPACE_BRANCH": spec.Branch, "WORKSPACE_TOKEN": f.grant(v), "WORKSPACE_CONNECTION_MODE": "blocked", "DATA_DIR": "/data", "INFERENCE_URL": "http://127.0.0.1:18081", "BROKER_URL": "http://127.0.0.1:18082", "EGRESS_URL": "http://127.0.0.1:18083", "WORKSPACE_AGENT": "codex", "CODEX_MODEL": "gpt-6.1-sol", "INFERENCE_MODEL": "fixture", "MONITOR_PROCESSES": "true", "WORKSPACE_T3": strconv.FormatBool(spec.AgentKind == "t3" || spec.ConsoleEnabled)}
	if isHermes {
		env["WORKSPACE_AGENT"] = "hermes"
		env["HERMES_MODEL"] = spec.HermesModel
		if spec.HermesMatrix != nil {
			b, _ := json.Marshal(spec.HermesMatrix)
			env["HERMES_MATRIX_CONFIG"] = string(b)
		}
	}
	b, _ := json.Marshal(env)
	if err = os.WriteFile(configDir+"/env.json", b, 0400); err != nil {
		return nil, err
	}
	boot := v.Jail + "/config.ext4"
	d, e := os.OpenFile(boot, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0400)
	if e != nil {
		return nil, e
	}
	e = d.Truncate(8 << 20)
	d.Close()
	if e != nil {
		return nil, e
	}
	if err = fcCommand(ctx, "mkfs.ext4", "-q", "-F", "-L", "awb-config", "-d", configDir, boot); err != nil {
		return nil, err
	}
	os.RemoveAll(configDir)
	b, _ = json.Marshal(fcConfig(1024))
	if err = os.WriteFile(v.Jail+"/config.json", b, 0400); err != nil {
		return nil, err
	}
	for _, p := range []string{boot, v.Jail + "/config.json"} {
		if err = os.Chown(p, next, next); err != nil {
			return nil, err
		}
	}
	if err = os.Chown(v.Jail, next, next); err != nil {
		return nil, err
	}
	if err = fcCommand(ctx, "ip", "netns", "add", v.NetNS); err != nil {
		return nil, err
	}
	if err = f.relays(v); err != nil {
		return nil, err
	}
	logFile, e := os.OpenFile(dir+"/console.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if e != nil {
		return nil, e
	}
	cmd := exec.Command(fcBase+"/jailer", "--id", generation, "--exec-file", fcBase+"/firecracker", "--uid", strconv.Itoa(next), "--gid", strconv.Itoa(next), "--chroot-base-dir", fcJails, "--netns", "/run/netns/"+v.NetNS, "--new-pid-ns", "--cgroup-version", "2", "--parent-cgroup", "awb-firecracker", "--cgroup", "memory.max=1610612736", "--cgroup", "memory.swap.max=0", "--cgroup", "pids.max=128", "--cgroup", "cpu.max=100000 100000", "--resource-limit", "no-file=256", "--resource-limit", "fsize=8589934592", "--", "--config-file", "/config.json", "--api-sock", "/api.sock")
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	console := &fcConsole{file: logFile, remaining: 64 << 20}
	cmd.Stdout = console
	cmd.Stderr = console
	cmd.Stdin = nil
	if err = cmd.Start(); err != nil {
		logFile.Close()
		return nil, err
	}
	f.mu.Lock()
	v.running = true
	f.vms[id] = v
	f.mu.Unlock()
	allocatedSlot = false
	f.allocation.Unlock()
	go func() { _ = cmd.Wait(); logFile.Close() }()
	err = waitForFirecrackerResume(ctx, func() error {
		return f.verifyVM(v)
	}, func() (int, error) {
		status, _, err := f.agent(ctx, v, "POST", "/v1/resume", nil)
		return status, err
	})
	if err != nil {
		return nil, err
	}
	if isHermes {
		status, b, probeErr := f.agent(ctx, v, "GET", "/v1/runtime-capabilities", nil)
		var c struct {
			Hermes bool `json:"hermes"`
		}
		if probeErr != nil || status != http.StatusOK || json.Unmarshal(b, &c) != nil || !c.Hermes {
			return nil, errors.New("Hermes guest runtime validation failed")
		}
	}
	return v, nil
}

func waitForFirecrackerResume(ctx context.Context, verify func() error, resume func() (int, error)) error {
	deadline := time.Now().Add(25 * time.Second)
	var lastCheck error
	for time.Now().Before(deadline) {
		lastCheck = verify()
		if lastCheck == nil {
			var status int
			status, lastCheck = resume()
			if lastCheck == nil {
				if status == http.StatusOK {
					return nil
				}
				lastCheck = fmt.Errorf("guest resume returned %d", status)
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w; last readiness check: %v", ctx.Err(), lastCheck)
		case <-time.After(200 * time.Millisecond):
		}
	}
	return fmt.Errorf("sandbox did not become ready: %w", lastCheck)
}

func checkFCSpec(a, b protocol.WorkspaceSpec) error {
	if a.ProjectID != b.ProjectID || a.HomeID != b.HomeID || a.AgentKind != b.AgentKind || a.TemplateID != b.TemplateID {
		return errors.New("workspace ownership mismatch")
	}
	return nil
}
func (f *Firecracker) kill(v *fcVM) error {
	f.mu.Lock()
	v.stopping = true
	v.Policy = shared.ConnectionPolicy{Mode: "blocked"}
	f.mu.Unlock()
	for _, l := range v.listeners {
		l.Close()
	}
	terminate := f.terminateCgroup
	if terminate == nil {
		terminate = terminateCgroup
	}
	if err := terminate(fcCgroups + "/" + v.Generation); err != nil {
		return err
	}
	f.mu.Lock()
	v.running = false
	v.stopping = false
	f.mu.Unlock()
	ctx, c := context.WithTimeout(context.Background(), 5*time.Second)
	defer c()
	if v.NetNS != "" {
		_ = fcCommand(ctx, "ip", "netns", "delete", v.NetNS)
	}
	return nil
}
func (f *Firecracker) verifyVM(v *fcVM) error {
	b, err := os.ReadFile(fcCgroups + "/" + v.Generation + "/cgroup.procs")
	if err != nil || len(strings.Fields(string(b))) != 1 {
		return errors.New("VMM cgroup identity unavailable")
	}
	pid := strings.TrimSpace(string(b))
	status, err := os.ReadFile("/proc/" + pid + "/status")
	if err != nil {
		return err
	}
	if !strings.Contains(string(status), fmt.Sprintf("Uid:\t%d\t%d\t%d\t%d", v.UID, v.UID, v.UID, v.UID)) || !strings.Contains(string(status), "Seccomp:\t2") {
		return errors.New("VMM privileges/seccomp verification failed")
	}
	for n, want := range map[string]string{"memory.max": "1610612736", "memory.swap.max": "0", "pids.max": "128", "cpu.max": "100000 100000"} {
		b, err := os.ReadFile(fcCgroups + "/" + v.Generation + "/" + n)
		if err != nil || strings.TrimSpace(string(b)) != want {
			return errors.New("VMM resource controls missing")
		}
	}
	// The jailer uses pivot_root in a private mount namespace. Its proc link
	// can display '/' even though it resolves to the jail's directory inode.
	// Compare filesystem identity, not a namespace-relative path string.
	root, err := os.Stat("/proc/" + pid + "/root")
	if err != nil {
		return fmt.Errorf("VMM root inspection failed: %w", err)
	}
	wantRoot, err := os.Stat(v.Jail)
	if err != nil || !os.SameFile(root, wantRoot) {
		return errors.New("VMM root directory identity mismatch")
	}
	a, err := os.Stat("/proc/" + pid + "/ns/net")
	if err != nil {
		return err
	}
	binfo, err := os.Stat("/run/netns/" + v.NetNS)
	if err != nil || !os.SameFile(a, binfo) {
		return errors.New("VMM network namespace mismatch")
	}
	// Query the actual running VMM: no guest NIC and no writable root drive.
	t := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", v.Jail+"/api.sock")
	}}
	defer t.CloseIdleConnections()
	client := &http.Client{Transport: t, Timeout: 2 * time.Second}
	resp, err := client.Get("http://vmm/vm/config")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var cfg struct {
		Networks []json.RawMessage `json:"network-interfaces"`
		Drives   []struct {
			ID string `json:"drive_id"`
			RO bool   `json:"is_read_only"`
		} `json:"drives"`
	}
	if resp.StatusCode != 200 || json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&cfg) != nil || len(cfg.Networks) != 0 || len(cfg.Drives) != 3 {
		return errors.New("VMM device configuration mismatch")
	}
	for _, d := range cfg.Drives {
		if (d.ID == "root" || d.ID == "config") && !d.RO {
			return errors.New("VMM system disk is writable")
		}
	}
	return nil
}
func (f *Firecracker) relays(v *fcVM) error {
	v.slots = make(chan struct{}, 32)
	if v.Spec.AgentKind == "hermes" {
		secret := projectMCPSecret()
		if secret == "" {
			return errors.New("project MCP signing authority unavailable")
		}
		l, err := net.Listen("unix", v.Jail+"/v.sock_18085")
		if err != nil {
			return err
		}
		v.listeners = append(v.listeners, l)
		if err = os.Chown(v.Jail+"/v.sock_18085", v.UID, v.UID); err != nil {
			return err
		}
		client := &http.Client{Timeout: 40 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		server := &http.Server{Handler: f.projectMCP(v, secret, client), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 45 * time.Second, MaxHeaderBytes: 16384}
		go server.Serve(&budgetListener{Listener: l, local: v.slots, global: f.slots})
	}

	if v.Spec.AgentKind == "hermes" && v.Spec.HermesMatrix != nil {
		l, err := net.Listen("unix", v.Jail+"/v.sock_18084")
		if err != nil {
			return err
		}
		v.listeners = append(v.listeners, l)
		if err = os.Chown(v.Jail+"/v.sock_18084", v.UID, v.UID); err != nil {
			return err
		}
		server := &http.Server{Handler: f.matrixProxy(v), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 45 * time.Second, MaxHeaderBytes: 16384}
		go server.Serve(&budgetListener{Listener: l, local: v.slots, global: f.slots})
	}
	for _, port := range []int{18081, 18082, 18083} {
		l, err := net.Listen("unix", fmt.Sprintf("%s/v.sock_%d", v.Jail, port))
		if err != nil {
			return err
		}
		v.listeners = append(v.listeners, l)
		_ = os.Chown(fmt.Sprintf("%s/v.sock_%d", v.Jail, port), v.UID, v.UID)
		go func(port int, l net.Listener) {
			for {
				c, err := l.Accept()
				if err != nil {
					return
				}
				select {
				case v.slots <- struct{}{}:
				default:
					c.Close()
					continue
				}
				select {
				case f.slots <- struct{}{}:
					go func() {
						defer func() { <-v.slots }()
						defer func() { <-f.slots }()
						defer c.Close()
						remote, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 3*time.Second)
						if err != nil {
							return
						}
						defer remote.Close()
						c.SetDeadline(time.Now().Add(10 * time.Minute))
						remote.SetDeadline(time.Now().Add(10 * time.Minute))
						go func() { io.Copy(remote, c); remote.Close() }()
						io.Copy(c, remote)
					}()
				default:
					<-v.slots
					c.Close()
				}
			}
		}(port, l)
	}
	return nil
}

type fcBufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *fcBufferedConn) Read(p []byte) (int, error) { return c.r.Read(p) }
func fcDial(ctx context.Context, v *fcVM) (net.Conn, error) {
	return fcDialPort(ctx, v, 8080)
}
func fcDialPort(ctx context.Context, v *fcVM, port int) (net.Conn, error) {
	c, err := (&net.Dialer{}).DialContext(ctx, "unix", v.Jail+"/v.sock")
	if err != nil {
		return nil, err
	}
	c.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err = fmt.Fprintf(c, "CONNECT %d\n", port); err != nil {
		c.Close()
		return nil, err
	}
	r := bufio.NewReaderSize(c, 256)
	line, err := r.ReadString('\n')
	if err != nil || len(line) > 64 || !strings.HasPrefix(line, "OK ") {
		c.Close()
		return nil, errors.New("guest vsock unavailable")
	}
	c.SetDeadline(time.Time{})
	return &fcBufferedConn{c, r}, nil
}
func (f *Firecracker) transport(v *fcVM) *http.Transport {
	return &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) { return fcDial(ctx, v) }, ResponseHeaderTimeout: 30 * time.Second}
}
func (f *Firecracker) agent(ctx context.Context, v *fcVM, method, path string, body io.Reader) (int, []byte, error) {
	t := f.transport(v)
	defer t.CloseIdleConnections()
	c := &http.Client{Transport: t, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	r, err := http.NewRequestWithContext(ctx, method, "http://guest"+path, body)
	if err != nil {
		return 0, nil, err
	}
	r.Header.Set("Authorization", "Bearer "+f.grant(v))
	r.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(r)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, protocol.AgentResponseLimit(path)+1))
	if int64(len(b)) > protocol.AgentResponseLimit(path) {
		return 0, nil, errors.New("workspace response too large")
	}
	return resp.StatusCode, b, err
}
func (f *Firecracker) observation(ctx context.Context, v *fcVM) shared.Observation {
	o := shared.Observation{RuntimeType: "firecracker", IsolationStatus: "unverified", NetworkStatus: "unverified", ConnectionMode: "blocked"}
	if v == nil {
		return o
	}
	f.mu.Lock()
	o.Running = v.running
	stopping := v.stopping
	o.ConnectionMode = v.Policy.Mode
	f.mu.Unlock()
	if !o.Running || stopping {
		return o
	}
	if f.verifyVM(v) != nil {
		_ = f.kill(v)
		f.mu.Lock()
		o.Running = v.running
		f.mu.Unlock()
		return o
	}
	// These statuses describe observed mechanisms, not a certification of immunity
	// to hypervisor/kernel vulnerabilities or provider data exfiltration.
	o.IsolationStatus = "jailed-vm"
	o.NetworkStatus = "no-nic-vsock"
	status, b, err := f.agent(ctx, v, "GET", "/v1/activity", nil)
	if err == nil && status == 200 && json.Unmarshal(b, &o.Activity) == nil {
		o.Ready = true
	}
	return o
}
func (f *Firecracker) Close() {
	f.lifecycle.Lock()
	defer f.lifecycle.Unlock()
	f.mu.Lock()
	vs := make([]*fcVM, 0, len(f.vms))
	for _, v := range f.vms {
		vs = append(vs, v)
	}
	f.mu.Unlock()
	for _, v := range vs {
		f.kill(v)
	}
}

func (f *Firecracker) Handler() http.Handler {
	return protocol.NewControllerHandler(protocol.Backend{
		Token: f.token, GatewayServiceToken: f.service, Health: f.health, VerifyGrant: f.verify,
		RegisterManagement: f.registerManagement,
		Start:              f.start, Stop: f.stop, ForceStop: f.forceStop, Inspect: f.inspectWorkspace,
		BindNetworkPolicy: f.bindNetworkPolicy, Reconcile: f.reconcileNetwork, Capabilities: f.capabilities,
		Proxy: f.proxy, Agent: f.proxyAgent,
		StartBodyLimit: 32768, AllowEmptyStart: true, WorkspaceActionFallback: true,
	})
}

func (f *Firecracker) registerManagement(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/projects/{id}/matrix-credential", f.storeMatrixCredential)
	mux.HandleFunc("GET /v1/projects/{id}/matrix-credential", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !shared.ValidID(id) {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		_, err := os.Stat(filepath.Join(matrixSecrets, id+".json"))
		shared.JSON(w, 200, map[string]bool{"configured": err == nil})
	})
	mux.HandleFunc("POST /v1/workspaces/{id}/matrix-enable", f.enableMatrixRoute)
	mux.HandleFunc("POST /v1/workspaces/{id}/matrix-disable", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		v := f.vms[r.PathValue("id")]
		if v != nil && f.matrixOwners[v.Spec.ProjectID] == v.Generation {
			delete(f.matrixOwners, v.Spec.ProjectID)
		}
		f.mu.Unlock()
		shared.JSON(w, 200, map[string]bool{"disabled": true})
	})
}

func (f *Firecracker) health(w http.ResponseWriter, r *http.Request) {
	shared.JSON(w, 200, map[string]string{"status": "ok", "backend": "firecracker"})
}

func (f *Firecracker) verify(w http.ResponseWriter, r *http.Request) {
	var q struct{ Token, Audience string }
	if !shared.Decode(w, r, &q, 2048) {
		return
	}
	if len(q.Token) != 64 || (q.Audience != "egress" && q.Audience != "inference" && q.Audience != "broker") {
		shared.Error(w, 403, "GRANT_REJECTED")
		return
	}
	f.mu.Lock()
	var found *fcVM
	for _, v := range f.vms {
		if v.running && !v.stopping && subtle.ConstantTimeCompare([]byte(f.grant(v)), []byte(q.Token)) == 1 {
			found = v
			break
		}
	}
	var p shared.ConnectionPolicy
	if found != nil {
		p = found.Policy
	}
	f.mu.Unlock()
	if found == nil || f.verifyVM(found) != nil || (q.Audience == "egress" && p.Mode == "blocked") {
		shared.Error(w, 403, "GRANT_REJECTED")
		return
	}
	shared.JSON(w, 200, map[string]any{"workspace_id": found.ID, "project_id": found.Spec.ProjectID, "principal_id": "local-operator", "connections": p})
}

func (f *Firecracker) capabilities() protocol.RuntimeCapability {
	caps := protocol.DevelopmentCapability("firecracker", "firecracker-2", 1<<30)
	caps.IsolationStatus = "jailed-vm"
	caps.NetworkStatus = "no-nic-vsock"
	if f.hermes && projectMCPEnabled() && projectMCPSecret() != "" {
		caps.AgentKinds = append(caps.AgentKinds, "hermes")
	}
	return caps
}

func (f *Firecracker) workspace(id string) *fcVM {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.vms[id]
}

func (f *Firecracker) inspectWorkspace(ctx context.Context, id string) (shared.Observation, *protocol.ControllerError) {
	if !shared.ValidID(id) {
		return shared.Observation{}, protocol.Failure(400, "INVALID_REQUEST")
	}
	return f.observation(ctx, f.workspace(id)), nil
}

var workspaceLockSeed = maphash.MakeSeed()

func (f *Firecracker) lockWorkspace(id string) func() {
	f.lifecycle.RLock()
	lock := &f.workspaceOps[maphash.String(workspaceLockSeed, id)%uint64(len(f.workspaceOps))]
	lock.Lock()
	return func() {
		lock.Unlock()
		f.lifecycle.RUnlock()
	}
}

func (f *Firecracker) start(ctx context.Context, id string, spec protocol.WorkspaceSpec) (shared.Observation, *protocol.ControllerError) {
	defer f.lockWorkspace(id)()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	v, err := f.launch(ctx, id, spec)
	if err != nil {
		slog.Error("Firecracker start refused", "workspace", id, "error", err)
		return shared.Observation{}, protocol.Failure(503, "FIRECRACKER_START_REFUSED")
	}
	return f.observation(ctx, v), nil
}

func (f *Firecracker) stop(ctx context.Context, id string) *protocol.ControllerError {
	defer f.lockWorkspace(id)()
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	f.mu.Lock()
	v := f.vms[id]
	running := v != nil && v.running && !v.stopping
	f.mu.Unlock()
	if running {
		status, _, err := f.agent(ctx, v, "POST", "/v1/quiesce", nil)
		if err != nil || status != 200 {
			return protocol.Failure(409, "WORKSPACE_BUSY")
		}
		if c, err := fcDialPort(ctx, v, 8081); err == nil {
			c.SetDeadline(time.Now().Add(3 * time.Second))
			_, err = c.Write([]byte("S"))
			var b [3]byte
			if err == nil {
				_, err = io.ReadFull(c, b[:])
			}
			c.Close()
			if err != nil || string(b[:]) != "OK\n" {
				return protocol.Failure(503, "SANDBOX_SYNC_FAILED")
			}
		} else {
			return protocol.Failure(503, "SANDBOX_SYNC_FAILED")
		}
	}
	if v != nil {
		if err := f.kill(v); err != nil {
			return protocol.Failure(503, "SANDBOX_STOP_FAILED")
		}
	}
	return nil
}

func (f *Firecracker) forceStop(_ context.Context, id string) *protocol.ControllerError {
	defer f.lockWorkspace(id)()
	if v := f.workspace(id); v != nil {
		if err := f.kill(v); err != nil {
			return protocol.Failure(503, "SANDBOX_STOP_FAILED")
		}
	}
	return nil
}

func (f *Firecracker) bindNetworkPolicy(_ context.Context, id string) func(*shared.ConnectionPolicy) *protocol.ControllerError {
	// Bind to the current VM before reading the request body so a delayed
	// update cannot follow a workspace ID to a replacement generation.
	v := f.workspace(id)
	return func(connections *shared.ConnectionPolicy) *protocol.ControllerError {
		return f.applyNetwork(v, connections)
	}
}

func (f *Firecracker) applyNetwork(v *fcVM, connections *shared.ConnectionPolicy) *protocol.ControllerError {
	if connections == nil {
		return protocol.Failure(400, "INVALID_REQUEST")
	}
	p := *connections
	if !p.Valid() || p.Mode == "open" {
		return protocol.Failure(403, "OPEN_NETWORK_FORBIDDEN_IN_SANDBOX")
	}
	f.mu.Lock()
	if v == nil || !v.running || v.stopping {
		f.mu.Unlock()
		return protocol.Failure(409, "NOT_READY")
	}
	v.Policy = p
	f.mu.Unlock()
	return nil
}

func (f *Firecracker) reconcileNetwork(_ context.Context, id string) *protocol.ControllerError {
	v := f.workspace(id)
	if v == nil || f.verifyVM(v) != nil {
		return protocol.Failure(503, "SANDBOX_ENFORCEMENT_UNAVAILABLE")
	}
	return nil
}

func (f *Firecracker) proxyAgent(ctx context.Context, id, method, path string, body io.Reader) (int, []byte, *protocol.ControllerError) {
	v := f.workspace(id)
	if v == nil || !f.observation(ctx, v).Ready {
		return 0, nil, protocol.Failure(503, "NOT_READY")
	}
	status, b, err := f.agent(ctx, v, method, path, body)
	if err != nil {
		return 0, nil, protocol.Failure(503, "WORKSPACE_UNAVAILABLE")
	}
	return status, b, nil
}

func (f *Firecracker) proxy(w http.ResponseWriter, r *http.Request, id, path, prefix string) {
	v := f.workspace(id)
	if v == nil || !f.observation(r.Context(), v).Ready {
		shared.Error(w, 503, "CONSOLE_NOT_READY")
		return
	}
	t := f.transport(v)
	defer t.CloseIdleConnections()
	p := &httputil.ReverseProxy{Transport: t, FlushInterval: -1, Rewrite: func(p *httputil.ProxyRequest) {
		p.Out.URL.Scheme = "http"
		p.Out.URL.Host = "guest"
		p.Out.URL.Path = prefix + path
		p.Out.URL.RawPath = ""
		p.Out.Host = p.In.Host
		p.Out.Header.Set("Authorization", "Bearer "+f.grant(v))
	}}
	p.ServeHTTP(w, r)
}
