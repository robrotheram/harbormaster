package lifecycle

import (
	"agent-workbench/internal/shared"
)

func CheckConnections(w shared.Workspace) error {
	p := w.EffectiveConnections()
	if w.RuntimeType == "firecracker" && p.Mode == "open" {
		return shared.CodedError{Status: shared.StatusConflict, Code: "OPEN_NETWORK_FORBIDDEN_IN_SANDBOX"}
	}
	if w.State != "STOPPED" && (p.Mode == "open") != (w.ConnectionMode == "open") {
		return shared.CodedError{Status: shared.StatusConflict, Code: "STOP_WORKSPACE_FIRST"}
	}
	return nil
}
