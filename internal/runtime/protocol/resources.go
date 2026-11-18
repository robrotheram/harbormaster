package protocol

import (
	"agent-workbench/internal/shared"
)

// CheckResources rejects a metadata/runtime disagreement before allocation.
// Omitted resources retain the fixed template for internal maintenance clients.
func CheckResources(requested *ResourceLimits, approved ResourceLimits) error {
	if requested != nil && *requested != approved {
		return shared.CodedError{Status: shared.StatusServiceUnavailable, Code: "RESOURCE_LIMIT_UNAVAILABLE"}
	}
	return nil
}
