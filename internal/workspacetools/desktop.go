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
	"sync"
	"syscall"
	"time"
)

func Desktop(ctx context.Context, args []string, errOut io.Writer) error {
	if len(args) != 0 {
		return errors.New("desktop takes no arguments")
	}
	port, err := strconv.Atoi(os.Getenv("PORT"))
	if err != nil || port < 1 || port > 65535 {
		return errors.New("PORT must be a valid TCP port")
	}
	env := []string{}
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "DISPLAY=") {
			env = append(env, entry)
		}
	}
	env = append(env, "DISPLAY=:99")
	var processes []*exec.Cmd
	var wait sync.WaitGroup
	exited := make(chan error, 5)
	defer func() {
		for _, p := range processes {
			_ = p.Process.Signal(syscall.SIGTERM)
		}
		done := make(chan struct{})
		go func() { wait.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			for _, p := range processes {
				_ = p.Process.Kill()
			}
			<-done
		}
	}()
	start := func(essential bool, name string, argv ...string) error {
		cmd := exec.Command(name, argv...)
		cmd.Env = env
		cmd.Stderr = errOut
		cmd.Stdout = errOut
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("start %s: %w", name, err)
		}
		processes = append(processes, cmd)
		wait.Add(1)
		go func() {
			defer wait.Done()
			err := cmd.Wait()
			if essential {
				exited <- fmt.Errorf("desktop process %s exited: %v", name, err)
			}
		}()
		return nil
	}
	ready := func(probe func() bool) error {
		for attempt := 0; attempt < 50; attempt++ {
			if probe() {
				return nil
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case err := <-exited:
				return err
			case <-time.After(100 * time.Millisecond):
			}
		}
		return errors.New("desktop display server did not become ready")
	}
	if err = start(true, "Xvfb", ":99", "-screen", "0", "1440x900x24", "-nolisten", "tcp", "-ac"); err != nil {
		return err
	}
	if err = ready(func() bool {
		probeCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer cancel()
		cmd := exec.CommandContext(probeCtx, "xdpyinfo")
		cmd.Env = env
		return cmd.Run() == nil
	}); err != nil {
		return err
	}
	if err = start(true, "fluxbox"); err != nil {
		return err
	}
	if err = start(true, "x11vnc", "-display", ":99", "-localhost", "-rfbport", "5901", "-nopw", "-forever", "-shared", "-quiet"); err != nil {
		return err
	}
	if err = ready(func() bool {
		conn, e := net.DialTimeout("tcp", "127.0.0.1:5901", 200*time.Millisecond)
		if e != nil {
			return false
		}
		conn.Close()
		return true
	}); err != nil {
		return err
	}
	if err = start(false, "xterm", "-title", "Workspace terminal", "-geometry", "100x30+35+35"); err != nil {
		return err
	}
	root, err := os.MkdirTemp("/tmp", "workbench-novnc.")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	assets, err := os.ReadDir("/usr/share/novnc")
	if err != nil {
		return err
	}
	for _, asset := range assets {
		if asset.Name() == "index.html" {
			continue
		}
		if err = os.Symlink(filepath.Join("/usr/share/novnc", asset.Name()), filepath.Join(root, asset.Name())); err != nil {
			return err
		}
	}
	if err = os.WriteFile(filepath.Join(root, "index.html"), []byte(`<!doctype html><meta http-equiv="refresh" content="0;url=vnc.html?autoconnect=1&amp;resize=scale&amp;path=websockify"><title>Workspace desktop</title>`), 0600); err != nil {
		return err
	}
	if err = start(true, "websockify", "--web="+root, "127.0.0.1:"+strconv.Itoa(port), "127.0.0.1:5901"); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return nil
	case err := <-exited:
		return err
	}
}
