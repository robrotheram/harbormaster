package protocol

import (
	"errors"

	"agent-workbench/internal/shared"
)

const DevelopmentTemplate = "development-v1"

// Runtime ownership comes from the authenticated control plane, never an agent.
func NormalizeSpec(spec WorkspaceSpec) (WorkspaceSpec, error) {
	if !shared.ValidMCPs(spec.MCPs) {
		return spec, errors.New("unsupported MCP selection")
	}
	if spec.ProjectID == "" {
		spec.ProjectID = "default"
	}
	if spec.AgentKind == "" {
		if spec.ProfileID != "" {
			spec.AgentKind = "t3"
		} else {
			spec.AgentKind = "codex"
		}
	}
	if spec.TemplateID == "" {
		spec.TemplateID = DevelopmentTemplate
	}
	if spec.ConnectionMode != "" && spec.ConnectionMode != "blocked" && spec.ConnectionMode != "allowlist" && spec.ConnectionMode != "open" {
		return spec, errors.New("unsupported connection mode")
	}
	if !shared.ValidID(spec.ProjectID) || (spec.HomeID != "" && !shared.ValidID(spec.HomeID)) || (spec.ProjectID != "default" && spec.HomeID == "") || spec.TemplateID != DevelopmentTemplate || !shared.ValidAgentKind(spec.AgentKind) {
		return spec, errors.New("unsupported workspace specification")
	}
	return spec, nil
}

func ProjectOf(labels map[string]string) string {
	if id := labels["workbench.project"]; id != "" {
		return id
	}
	return "default"
}

func CheckOwnership(labels map[string]string, spec WorkspaceSpec) error {
	if labels["workbench.project"] == "" {
		if spec.ProjectID == "default" && spec.HomeID == "" {
			return nil
		}
		return errors.New("legacy ownership cannot be reassigned")
	}
	if labels["workbench.project"] != spec.ProjectID || labels["workbench.home"] != spec.HomeID {
		return errors.New("workspace ownership mismatch")
	}
	return nil
}
