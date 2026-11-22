package setup

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"agent-workbench/internal/egress"
	"agent-workbench/internal/shared"
)

func (s Service) command(ctx context.Context, dir, executable string, args ...string) (string, error) {
	state := Dir(s.Home)
	home := filepath.Join(state, "cli-home")
	if os.MkdirAll(home, 0700) != nil {
		return "", ErrStorage
	}
	tmp, err := os.MkdirTemp(state, "cli-tmp-")
	if err != nil {
		return "", ErrStorage
	}
	defer os.RemoveAll(tmp)
	// Use dedicated CLI homes and never inherit the supervisor's grants or Git
	// configuration. The staged directory has only configuration we validated.
	env := []string{
		"PATH=" + BasePath, "HOME=" + home, "LANG=C.UTF-8", "TMPDIR=" + tmp,
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=protocol.file.allow", "GIT_CONFIG_VALUE_0=never",
		"MISE_DATA_DIR=" + filepath.Join(state, "mise-data"),
		"MISE_GLOBAL_CONFIG_FILE=/etc/harbormaster-no-mise.toml", "MISE_SYSTEM_CONFIG_FILE=/etc/harbormaster-no-mise.toml",
		"MISE_CEILING_PATHS=" + state,
		"MISE_OVERRIDE_CONFIG_FILENAMES=mise.toml:.mise.toml",
		"MISE_YES=1", "MISE_AUTO_INSTALL=0", "MISE_FETCH_REMOTE_VERSIONS_TIMEOUT=30s",
		"MISE_DISABLE_BACKENDS=asdf,vfox", "MISE_USE_TOML=1", "DISABLE_TELEMETRY=1", "NO_COLOR=1",
	}
	if s.Proxy {
		env = egress.Environment(env)
	}

	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir, cmd.Env = dir, env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = time.Second
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	stdout, stderr := shared.NewCappedOutput(512<<10), shared.NewCappedOutput(128<<10)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Start(); err != nil {
		return "", ErrUnavailable
	}
	err = cmd.Wait()
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if ctx.Err() != nil {
		return "", ErrInterrupted
	}
	if err != nil {
		if errors.Is(err, exec.ErrWaitDelay) {
			return "", ErrInterrupted
		}
		diagnostic := strings.ToLower(string(stderr.Data))
		if s.Proxy && (s.Blocked || strings.Contains(diagnostic, "connect tunnel failed, response 403") ||
			strings.Contains(diagnostic, "connection denied by workspace policy")) {
			return "", ErrNetwork
		}
		return "", ErrDownload
	}
	return string(stdout.Data), nil
}
