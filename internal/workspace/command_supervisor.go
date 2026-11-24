package workspace

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const commandSupervisorArg = "--workspace-command-supervisor"

// CommandSupervisor handles the workspace binary's internal subprocess mode.
// The dedicated process adopts orphaned descendants without affecting other jobs.
func CommandSupervisor(args []string) (int, bool) {
	if len(args) != 2 || args[0] != commandSupervisorArg {
		return 0, false
	}
	if unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0) != nil {
		return 125, true
	}
	stopping := make(chan os.Signal, 1)
	signal.Notify(stopping, syscall.SIGTERM)
	defer signal.Stop(stopping)
	cmd := exec.Command("/bin/sh", "-c", args[1])
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return 125, true
	}
	result := make(chan error, 1)
	go func() { result <- cmd.Wait() }()
	var err error
	select {
	case err = <-result:
	case <-stopping:
		_ = cmd.Process.Kill()
		err = <-result
	}
	if !reapCommandChildren() {
		return 125, true
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() >= 0 {
		return exit.ExitCode(), true
	}
	if err != nil {
		return 125, true
	}
	return 0, true
}

func reapCommandChildren() bool {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		pid, err := unix.Wait4(-1, nil, unix.WNOHANG, nil)
		switch {
		case errors.Is(err, unix.ECHILD):
			return true
		case errors.Is(err, unix.EINTR), pid > 0:
			continue
		case err != nil:
			return false
		}
		threads, err := os.ReadDir("/proc/self/task")
		if err != nil {
			return false
		}
		for _, thread := range threads {
			children, err := os.ReadFile(filepath.Join("/proc/self/task", thread.Name(), "children"))
			if err != nil {
				continue
			}
			for _, child := range strings.Fields(string(children)) {
				// Wait4 runs on this loop only, so an unreaped child PID
				// cannot be reassigned before we send the signal.
				if pid, err := strconv.Atoi(child); err == nil && pid > 1 {
					_ = unix.Kill(pid, unix.SIGKILL)
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}
