package firecracker

import (
	"errors"
	"os"
	"strings"
	"time"
)

// terminateCgroup targets a generation, never an unverified or reused host PID.
// A missing cgroup confirms absence; any other observation failure keeps the
// VM in stopping state, with its grants revoked and its capacity reserved.
func terminateCgroup(group string) error {
	if err := os.WriteFile(group+"/cgroup.kill", []byte("1"), 0600); err != nil && !os.IsNotExist(err) {
		return err
	}
	for i := 0; i < 100; i++ {
		b, err := os.ReadFile(group + "/cgroup.procs")
		if os.IsNotExist(err) || (err == nil && len(strings.Fields(string(b))) == 0) {
			return nil
		}
		if err != nil {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
	return errors.New("VMM did not terminate; data writer remains fenced")
}
