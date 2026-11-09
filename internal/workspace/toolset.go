package workspace

import (
	"agent-workbench/internal/workspace/filetools"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
)

// NewToolset reuses confined file operations without opening the task database or
// performing execution recovery. It is used by workspace-local MCP processes.
func NewToolset(dataDir string) (func(context.Context, string, json.RawMessage) string, io.Closer, error) {
	root, err := os.OpenRoot(filepath.Join(dataDir, "files"))
	if err != nil {
		return nil, nil, err
	}
	tools := filetools.New(root)
	return tools.Call, root, nil
}
