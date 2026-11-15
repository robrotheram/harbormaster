package workspacetools

import (
	"fmt"
	"os"
	"syscall"
)

// Hermes is an upstream Python SDK. Its workspace adapter is an installed
// runtime package; the compiled launcher preserves the supervised process ID.
func Hermes(name string, args []string) error {
	module, ok := map[string]string{"hermes-runner": "runner", "hermes-matrix": "matrix", "hermes-digest": "digest", "hermes-consume": "consumer"}[name]
	if !ok {
		return fmt.Errorf("unknown Hermes entrypoint")
	}
	python := "/opt/hermes/.venv/bin/python"
	return syscall.Exec(python, append([]string{python, "-m", "workbench_hermes." + module}, args...), os.Environ())
}
