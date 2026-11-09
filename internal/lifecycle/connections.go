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
	if w.State != "STOPPED" && (((p.Mode == "open") != (w.ConnectionMode == "open")) || (w.ConnectionProxyVersion < 2 && p.Mode == "allowlist" && !(len(p.Destinations) == 1 && p.Allows("github.com:443")))) {
		return errors.New("STOP_WORKSPACE_FIRST")
	}
	return nil
}
