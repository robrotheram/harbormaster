package toolconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

func SyncCodex(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := []string{}
	skip := false
	for _, line := range strings.Split(string(b), "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "[") {
			skip = strings.HasPrefix(trim, "[mcp_servers.workspace-files]") || strings.HasPrefix(trim, "[mcp_servers.project-git]") || strings.HasPrefix(trim, "[mcp_servers.workspace-files.") || strings.HasPrefix(trim, "[mcp_servers.project-git.")
		}
		if !skip {
			lines = append(lines, line)
		}
	}
	content := strings.TrimSpace(strings.Join(lines, "\n")) + "\n"
	content += CodexTOML()
	temp, err := os.CreateTemp(filepath.Dir(path), "config-*.toml")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err = temp.WriteString(content); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}

func tomlString(s string) string { b, _ := json.Marshal(s); return string(b) }

// CodexTOML is shared by initial T3 configuration and restoration of the pinned
// local tools in an existing private native home.
func CodexTOML() string {
	var content strings.Builder
	for _, entry := range Bundled() {
		content.WriteString("\n[mcp_servers." + entry.Name + "]\ncommand = " + tomlString(Command) + "\nargs = [\"--mode\", " + tomlString(entry.Mode) + "]\n")
	}
	return content.String()
}
