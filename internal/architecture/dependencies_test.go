// Package architecture guards the dependency boundaries documented in docs/architecture.md.
package architecture

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// These rules cover production imports, so boundary tests can still assemble
// actual adapters. Keep interfaces with their consumers and prevent application
// policy from depending on a concrete transport or persistence implementation.
func TestModuleDependencies(t *testing.T) {
	root := filepath.Join("..", "..", "internal")
	rules := map[string][]string{
		"lifecycle":                {"net/http", "database/sql", "os/exec", "agent-workbench/internal/api", "agent-workbench/internal/store", "agent-workbench/internal/runtime", "agent-workbench/internal/workspace"},
		"workspacesvc":             {"net/http", "database/sql", "agent-workbench/internal/api", "agent-workbench/internal/store", "agent-workbench/internal/workspace", "agent-workbench/internal/runtime"},
		"orchestration":            {"net/http", "agent-workbench/internal/api", "agent-workbench/internal/dashboard", "agent-workbench/internal/mcpserver", "agent-workbench/internal/runtime", "agent-workbench/internal/workspace"},
		"runtime/protocol":         {"net/http", "os/exec", "agent-workbench/internal/api", "agent-workbench/internal/store", "agent-workbench/internal/workspace", "agent-workbench/internal/runtime"},
		"runtime/controllerclient": {"agent-workbench/internal/api", "agent-workbench/internal/store", "agent-workbench/internal/workspace", "agent-workbench/internal/runtime/docker", "agent-workbench/internal/runtime/kubernetes", "agent-workbench/internal/runtime/firecracker"},
		"runtime/docker":           {"agent-workbench/internal/api", "agent-workbench/internal/store", "agent-workbench/internal/workspace", "agent-workbench/internal/runtime/kubernetes", "agent-workbench/internal/runtime/firecracker"},
		"runtime/kubernetes":       {"agent-workbench/internal/api", "agent-workbench/internal/store", "agent-workbench/internal/workspace", "agent-workbench/internal/runtime/docker", "agent-workbench/internal/runtime/firecracker"},
		"runtime/firecracker":      {"agent-workbench/internal/api", "agent-workbench/internal/store", "agent-workbench/internal/workspace", "agent-workbench/internal/runtime/docker", "agent-workbench/internal/runtime/kubernetes"},
		"workspace/filetools":      {"net/http", "database/sql", "agent-workbench/internal/api", "agent-workbench/internal/store", "agent-workbench/internal/runtime", "agent-workbench/internal/workspace"},
		"workspace/nativeagent":    {"net/http", "database/sql", "agent-workbench/internal/api", "agent-workbench/internal/store", "agent-workbench/internal/runtime", "agent-workbench/internal/workspace"},
		"workspace/taskstore":      {"net/http", "os/exec", "agent-workbench/internal/api", "agent-workbench/internal/store", "agent-workbench/internal/runtime", "agent-workbench/internal/workspace"},
	}
	for module, forbidden := range rules {
		t.Run(module, func(t *testing.T) {
			err := filepath.WalkDir(filepath.Join(root, filepath.FromSlash(module)), func(path string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
					return nil
				}
				file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
				if err != nil {
					return err
				}
				for _, imp := range file.Imports {
					name, err := strconv.Unquote(imp.Path.Value)
					if err != nil {
						return err
					}
					// Protocol is the provider-neutral wire vocabulary, not a provider.
					if name == "agent-workbench/internal/runtime/protocol" {
						continue
					}
					for _, prefix := range forbidden {
						if name == prefix || strings.HasPrefix(name, prefix+"/") {
							t.Errorf("%s imports %s across the %s boundary", path, name, module)
						}
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
