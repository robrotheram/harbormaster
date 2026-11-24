package workspacetools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const desktopSocket = "wayland-0"

func desktopRuntimeDir() string {
	return fmt.Sprintf("/tmp/workbench-desktop-%d", os.Getuid())
}

func checkDesktopRuntimeDir(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm() != 0700 || info.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) {
		return errors.New("desktop runtime directory must be private and owned by the workspace user")
	}
	return nil
}

func desktopListening(network, address string) bool {
	conn, err := net.DialTimeout(network, address, 200*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// DesktopEnvironment also works before a desktop starts. The X11 display is
// allocated by wland at startup, so launchers resolve it again when needed.
func DesktopEnvironment() []string {
	runtimeDir := desktopRuntimeDir()
	socket := filepath.Join(runtimeDir, desktopSocket)
	display := ""
	if desktopListening("unix", socket) {
		if data, err := os.ReadFile(socket + ".x11"); err == nil {
			n, ok := strings.CutPrefix(strings.TrimSpace(string(data)), ":")
			if number, err := strconv.ParseUint(n, 10, 32); ok && err == nil {
				display = ":" + strconv.FormatUint(number, 10)
			}
		}
	}
	return []string{
		"XDG_RUNTIME_DIR=" + runtimeDir,
		"WAYLAND_DISPLAY=" + desktopSocket,
		"XDG_SESSION_TYPE=wayland",
		"GSK_RENDERER=cairo",
		"DISPLAY=" + display,
	}
}

func Desktop(ctx context.Context, args []string, errOut io.Writer) error {
	if len(args) != 0 {
		if len(args) < 2 || args[0] != "run" {
			return errors.New("usage: workbench-desktop run <app> [args...]")
		}
		return requestDesktopLaunch(ctx, args[1:])
	}

	port, err := strconv.Atoi(os.Getenv("PORT"))
	if err != nil || port < 1 || port > 65535 {
		return errors.New("PORT must be a valid TCP port")
	}
	runtimeDir := desktopRuntimeDir()
	if err := os.Mkdir(runtimeDir, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create desktop runtime directory: %w", err)
	}
	if err := checkDesktopRuntimeDir(runtimeDir); err != nil {
		return fmt.Errorf("inspect desktop runtime directory: %w", err)
	}
	listener, err := listenDesktopLaunches(runtimeDir)
	if err != nil {
		return fmt.Errorf("listen for desktop launches: %w", err)
	}
	defer listener.Close()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	relay := exec.CommandContext(ctx, "/usr/local/bin/wlandr",
		"--profile", "session", "--host", "127.0.0.1", "--port", strconv.Itoa(port),
		"--display", desktopSocket, "--web-root", "/opt/wland/web",
		"--xwayland", "/usr/local/bin/xwayland-satellite")
	relay.Env = append(os.Environ(), "XDG_RUNTIME_DIR="+runtimeDir)
	relay.Stdout, relay.Stderr = errOut, errOut
	relay.Cancel = func() error { return relay.Process.Signal(syscall.SIGTERM) }
	relay.WaitDelay = 2 * time.Second
	if err := relay.Start(); err != nil {
		return fmt.Errorf("start desktop compositor: %w", err)
	}
	exited := make(chan struct{})
	var relayErr error
	go func() {
		relayErr = relay.Wait()
		close(exited)
	}()
	defer func() {
		cancel()
		<-exited
	}()

	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	socket := filepath.Join(runtimeDir, desktopSocket)
	for !desktopListening("tcp", address) || !desktopListening("unix", socket) {
		select {
		case <-ctx.Done():
			return nil
		case <-exited:
			return fmt.Errorf("desktop compositor exited during startup: %v", relayErr)
		case <-deadline.C:
			return errors.New("desktop compositor did not become ready")
		case <-tick.C:
		}
	}

	go serveDesktopLaunches(ctx, listener, errOut)

	launch := desktopLaunch{Args: []string{"/usr/bin/foot", "--title", "Workspace terminal"}, Env: os.Environ()}
	terminalExited, err := launch.start(ctx, errOut)
	if err != nil {
		return fmt.Errorf("start workspace terminal: %w", err)
	}
	defer func() { cancel(); <-terminalExited }()
	select {
	case <-ctx.Done():
		return nil
	case <-exited:
		return fmt.Errorf("desktop compositor exited: %v", relayErr)
	}
}
