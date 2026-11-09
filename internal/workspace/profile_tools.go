package workspace

import (
	"os"
	"strings"
)

type mcpEntry struct{ name, mode string }

func (m *Manager) mcpEntries() []mcpEntry {
	names := m.cfg.MCPs
	if names == nil {
		names = []string{"workspace-files", "project-git"}
	}
	entries := []mcpEntry{}
	for _, name := range names {
		mode := "files"
		if name == "project-git" {
			mode = "git"
		}
		entries = append(entries, mcpEntry{name, mode})
	}
	return entries
}
func (m *Manager) syncMCPConfig(path string) error {
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
	for _, entry := range m.mcpEntries() {
		content += "\n[mcp_servers." + entry.name + "]\ncommand = \"/usr/local/bin/workspace-mcp\"\nargs = [\"--mode\", " + tomlString(entry.mode) + "]\n"
	}
	temp, err := os.CreateTemp(strings.TrimSuffix(path, "/config.toml"), "config-*.toml")
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
