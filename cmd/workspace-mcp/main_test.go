package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPProcessHelper(t *testing.T) {
	if os.Getenv("WORKBENCH_MCP_TEST_HELPER") != "1" {
		return
	}
	os.Args = []string{"workspace-mcp", "--mode", os.Getenv("WORKBENCH_MCP_TEST_MODE")}
	main()
	os.Exit(0)
}
func TestLocalMCPServers(t *testing.T) {
	data := t.TempDir()
	os.Mkdir(filepath.Join(data, "files"), 0700)
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"files", "git"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			command := exec.Command(binary, "-test.run=TestMCPProcessHelper")
			command.Env = append(os.Environ(), "WORKBENCH_MCP_TEST_HELPER=1", "WORKBENCH_MCP_TEST_MODE="+mode, "DATA_DIR="+data)
			client, err := mcp.NewClient(&mcp.Implementation{Name: "workspace-integration", Version: "1"}, nil).Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			list, err := client.ListTools(ctx, nil)
			if err != nil || len(list.Tools) != 3 {
				t.Fatal("workspace MCP tools unavailable")
			}
			if mode == "files" {
				result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "write_file", Arguments: map[string]string{"path": "example.txt", "content": "workspace content"}})
				if err != nil || result.IsError {
					t.Fatal("file MCP write failed")
				}
				result, err = client.CallTool(ctx, &mcp.CallToolParams{Name: "read_file", Arguments: map[string]string{"path": "../outside"}})
				if err != nil {
					t.Fatal(err)
				}
				encoded, _ := json.Marshal(result.StructuredContent)
				if !strings.Contains(string(encoded), "FILE_UNAVAILABLE") {
					t.Fatal("file MCP traversal accepted")
				}
			}
		})
	}
}
