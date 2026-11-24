package firecracker

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

var generationName = regexp.MustCompile(`^fc-[a-f0-9]{32}$`)

func terminateLauncher(process *os.Process) error {
	if process == nil {
		return nil
	}
	if err := process.Kill(); err != nil {
		if errors.Is(err, os.ErrProcessDone) {
			return nil
		}
		return err
	}
	// cmd.Wait reaps the process before it finishes draining the logging
	// pipes. A live descendant can still have those pipes open.
	for i := 0; i < 100; i++ {
		if err := process.Signal(syscall.Signal(0)); err != nil {
			if errors.Is(err, os.ErrProcessDone) {
				return nil
			}
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
	return errors.New("Firecracker launcher did not exit")
}

func removeGenerationJail(parent, generation string) error {
	if !generationName.MatchString(generation) {
		return errors.New("invalid Firecracker generation")
	}
	root, err := os.OpenRoot(parent)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	// The workspace disk resides outside this tree. Its jail hard link is
	// removed along with the generation-specific config image and sockets.
	return root.RemoveAll(generation)
}

func reapGenerations(cgroups, jails string, terminate func(string) error) error {
	generations := make(map[string]struct{})
	for _, dir := range []string{cgroups, jails} {
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) && dir == jails {
			continue
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.IsDir() && generationName.MatchString(entry.Name()) {
				generations[entry.Name()] = struct{}{}
			}
		}
	}
	for generation := range generations {
		if err := terminate(filepath.Join(cgroups, generation)); err != nil {
			return fmt.Errorf("terminate generation %s: %w", generation, err)
		}
		if err := removeGenerationJail(jails, generation); err != nil {
			return fmt.Errorf("remove jail for generation %s: %w", generation, err)
		}
	}
	return nil
}

// terminateCgroup targets a generation, never an unverified or reused host PID.
// A missing cgroup confirms absence; any other observation failure keeps the
// VM in stopping state, with its grants revoked and its capacity reserved.
func terminateCgroup(group string) error {
	if err := os.WriteFile(group+"/cgroup.kill", []byte("1"), 0600); err != nil && !os.IsNotExist(err) {
		return err
	}
	for i := 0; i < 100; i++ {
		b, err := os.ReadFile(group + "/cgroup.events")
		if os.IsNotExist(err) || (err == nil && strings.Contains("\n"+string(b), "\npopulated 0\n")) {
			return nil
		}
		if err != nil {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
	return errors.New("VMM did not terminate; data writer remains fenced")
}
