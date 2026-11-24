package workspacetools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const desktopLaunchSocket = "launch.sock"

type desktopLaunch struct {
	Args []string
	Dir  string
	Env  []string
}

func (launch desktopLaunch) start(ctx context.Context, output io.Writer) (<-chan error, error) {
	cmd := exec.CommandContext(ctx, "/usr/bin/dbus-run-session", append([]string{"--"}, launch.Args...)...)
	cmd.Dir = launch.Dir
	env := slices.DeleteFunc(launch.Env, func(entry string) bool {
		return strings.HasPrefix(entry, "WAYLAND_SOCKET=") || strings.HasPrefix(entry, "DBUS_SESSION_BUS_ADDRESS=")
	})
	cmd.Env = append(env, DesktopEnvironment()...)
	cmd.Stdout, cmd.Stderr = output, output
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	return exited, nil
}

func listenDesktopLaunches(runtimeDir string) (net.Listener, error) {
	path := filepath.Join(runtimeDir, desktopLaunchSocket)
	if desktopListening("unix", path) {
		return nil, errors.New("desktop launcher is already running")
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("clear desktop launcher socket: %w", err)
	}
	return net.Listen("unix", path)
}

func requestDesktopLaunch(ctx context.Context, args []string) error {
	executable, err := exec.LookPath(args[0])
	if err != nil {
		return err
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return err
	}
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	runtimeDir := desktopRuntimeDir()
	if err := checkDesktopRuntimeDir(runtimeDir); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return errors.New("desktop is unavailable. Start it with workbench-preview desktop")
	}
	dialer := net.Dialer{Timeout: time.Second}
	conn, err := dialer.DialContext(ctx, "unix", filepath.Join(runtimeDir, desktopLaunchSocket))
	if err != nil {
		return errors.New("desktop launcher is unavailable. Start or restart the desktop session")
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	launch := desktopLaunch{Args: append([]string{executable}, args[1:]...), Dir: dir, Env: os.Environ()}
	if err := json.NewEncoder(conn).Encode(launch); err != nil {
		return err
	}
	var message string
	if err := json.NewDecoder(io.LimitReader(conn, 4096)).Decode(&message); err != nil {
		return fmt.Errorf("read desktop launch response: %w", err)
	}
	if message != "" {
		return errors.New(message)
	}
	return nil
}

// Supervise apps independently of the launching shell or agent run.
func serveDesktopLaunches(ctx context.Context, listener net.Listener, output io.Writer) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		message := ""
		if err := launchDesktopApp(ctx, conn, output); err != nil {
			message = err.Error()
		}
		conn.SetWriteDeadline(time.Now().Add(time.Second))
		_ = json.NewEncoder(conn).Encode(message)
		conn.Close()
	}
}

func launchDesktopApp(ctx context.Context, input io.Reader, output io.Writer) error {
	var launch desktopLaunch
	if err := json.NewDecoder(io.LimitReader(input, 1<<20)).Decode(&launch); err != nil || len(launch.Args) == 0 || !filepath.IsAbs(launch.Args[0]) || !filepath.IsAbs(launch.Dir) {
		return errors.New("invalid desktop launch request")
	}
	exited, err := launch.start(ctx, output)
	if err != nil {
		return err
	}
	name := filepath.Base(launch.Args[0])
	// Report immediate startup failures. Long-running apps log later exits.
	startup := time.NewTimer(250 * time.Millisecond)
	defer startup.Stop()
	select {
	case err := <-exited:
		if err != nil {
			return fmt.Errorf("desktop app %s exited during startup: %w", name, err)
		}
	case <-ctx.Done():
		return ctx.Err()
	case <-startup.C:
		go func() {
			if err := <-exited; err != nil && ctx.Err() == nil {
				fmt.Fprintf(output, "desktop app %s exited: %v\n", name, err)
			}
		}()
	}
	return nil
}
