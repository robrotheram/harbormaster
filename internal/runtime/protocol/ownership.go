package protocol

import (
	"errors"

	"agent-workbench/internal/shared"
)

const DevelopmentTemplate = "development-v1"

// Runtime ownership comes from the authenticated control plane, never an agent.
func NormalizeSpec(spec WorkspaceSpec) (WorkspaceSpec, error) {
	if spec.ProjectID == "" {
		spec.ProjectID = "default"
	}
	if spec.AgentKind == "" {
		spec.AgentKind = "codex"
	}
	if spec.TemplateID == "" {
		spec.TemplateID = DevelopmentTemplate
	}
	if spec.ConnectionMode != "" && spec.ConnectionMode != "blocked" && spec.ConnectionMode != "allowlist" && spec.ConnectionMode != "open" {
		return spec, errors.New("unsupported connection mode")
	}
	if !shared.ValidID(spec.ProjectID) || !shared.ValidID(spec.HomeID) || spec.TemplateID != DevelopmentTemplate || !shared.ValidAgentKind(spec.AgentKind) {
		return spec, errors.New("unsupported workspace specification")
	}
	return spec, nil
}

func HasOwnershipLabels(labels map[string]string) bool {
	return shared.ValidID(labels["workbench.project"]) && shared.ValidID(labels["workbench.home"])
}

func CheckOwnership(labels map[string]string, spec WorkspaceSpec) error {
	if !HasOwnershipLabels(labels) || labels["workbench.project"] != spec.ProjectID || labels["workbench.home"] != spec.HomeID {
		return errors.New("workspace ownership mismatch")
	}
	return nil
}
