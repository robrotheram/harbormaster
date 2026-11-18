package lifecycle

import (
	"agent-workbench/internal/shared"
	"errors"
)

func CheckConnections(w shared.Workspace) error {
	p := w.EffectiveConnections()
	if w.RuntimeType == "firecracker" && p.Mode == "open" {
		return errors.New("OPEN_NETWORK_FORBIDDEN_IN_SANDBOX")
	}
	if w.State != "STOPPED" && (p.Mode == "open") != (w.ConnectionMode == "open") {
		return errors.New("STOP_WORKSPACE_FIRST")
	}
	return nil
}
