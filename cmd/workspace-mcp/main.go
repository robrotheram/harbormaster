package main

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"agent-workbench/internal/shared"
	"agent-workbench/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	mode := flag.String("mode", "files", "files or git")
	flag.Parse()
	data := shared.Env("DATA_DIR", "/data")
	call, closer, err := workspace.NewToolset(data)
	if err != nil {
		os.Exit(1)
	}
	defer closer.Close()
	server := mcp.NewServer(&mcp.Implementation{Name: "workspace-" + *mode, Version: "1.0.0"}, nil)
	type fileInput struct {
		Path    string `json:"path"`
		Content string `json:"content,omitempty"`
	}
	if *mode == "files" {
		for _, name := range []string{"read_file", "write_file", "list_files"} {
			mcp.AddTool(server, &mcp.Tool{Name: name, Description: "Confined workspace file operation, maximum 64 KiB."}, func(ctx context.Context, _ *mcp.CallToolRequest, in fileInput) (*mcp.CallToolResult, map[string]any, error) {
				raw, _ := json.Marshal(in)
				result := map[string]any{}
				_ = json.Unmarshal([]byte(call(ctx, name, raw)), &result)
				return nil, result, nil
			})
		}
	} else if *mode == "git" {
		commands := map[string][]string{"git_status": {"status", "--porcelain=v1", "--branch"}, "git_diff": {"diff", "--no-ext-diff", "--no-textconv", "--stat"}, "git_log": {"log", "-10", "--oneline", "--no-decorate"}}
		for name, args := range commands {
			mcp.AddTool(server, &mcp.Tool{Name: name, Description: "Read project Git state; no remote access or mutations."}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, map[string]any, error) {
				ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, "git", args...)
				cmd.Dir = filepath.Join(data, "files")
				cmd.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=" + cmd.Dir, "GIT_TERMINAL_PROMPT=0"}
				out := &bounded{}
				cmd.Stdout = out
				cmd.Stderr = out
				err := cmd.Run()
				return nil, map[string]any{"output": string(out.b), "succeeded": err == nil}, nil
			})
		}
	} else {
		os.Exit(1)
	}
	if server.Run(context.Background(), &mcp.StdioTransport{}) != nil {
		os.Exit(1)
	}
}

type bounded struct{ b []byte }

func (b *bounded) Write(p []byte) (int, error) {
	n := len(p)
	if rem := 65536 - len(b.b); rem > 0 {
		b.b = append(b.b, p[:min(rem, n)]...)
	}
	return n, nil
}
