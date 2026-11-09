// Package toolconfig defines the workspace's fixed, local file and read-only
// Git tools. It does not accept user-selected servers or credentials.
package toolconfig

type Server struct{ Name, Mode string }

const Command = "/usr/local/bin/workspace-mcp"

// Bundled returns a value copy so callers cannot change the process-wide bundle.
func Bundled() [2]Server {
	return [2]Server{{Name: "workspace-files", Mode: "files"}, {Name: "project-git", Mode: "git"}}
}
