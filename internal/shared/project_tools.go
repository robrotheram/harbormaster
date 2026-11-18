package shared

import (
	_ "embed"
	"encoding/json"
	"slices"
)

// ProjectToolPolicy is the neutral fixed tool policy embedded by both brokers
// and installed beside the Hermes adapter. No administrative tools are admitted.
//
//go:embed project-tools.json
var projectToolPolicy []byte

func ProjectToolAllowed(name string) bool {
	var names []string
	if json.Unmarshal(projectToolPolicy, &names) != nil {
		return false
	}
	return slices.Contains(names, name)
}
