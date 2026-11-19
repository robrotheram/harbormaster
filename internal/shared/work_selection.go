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

// ValidReasoningEffort accepts native CLI settings. Empty uses the agent default.
func ValidReasoningEffort(provider, effort string) bool {
	if effort == "" {
		return true
	}
	switch provider {
	case "codex":
		switch effort {
		case "minimal", "low", "medium", "high", "xhigh", "max":
			return true
		}
	case "claude":
		switch effort {
		case "low", "medium", "high", "xhigh", "max":
			return true
		}
	}
	return false
}
