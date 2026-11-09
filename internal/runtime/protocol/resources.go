package protocol

import "errors"

// CheckResources rejects a metadata/runtime disagreement before allocation.
// Omitted resources retain the fixed template for internal maintenance clients.
func CheckResources(requested *ResourceLimits, approved ResourceLimits) error {
	if requested != nil && *requested != approved {
		return errors.New("RESOURCE_LIMIT_UNAVAILABLE")
	}
	return nil
}
