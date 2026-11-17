package shared

import "strings"

// ValidWorkSelection accepts native agents and bounded model identifiers.
// Provider gateways enforce their configured model allowlists at inference time.
func ValidWorkSelection(provider, model string) bool {
	if provider != "" && provider != "codex" && provider != "claude" {
		return false
	}
	if model == "" {
		return true
	}
	if provider == "" || len(model) > 200 || strings.TrimSpace(model) != model {
		return false
	}
	for _, c := range model {
		if c <= 32 || c > 126 {
			return false
		}
	}
	return true
}
