package workspace

import (
	"agent-workbench/internal/egress"
	"agent-workbench/internal/shared"
	"agent-workbench/internal/workspace/filetools"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

func toolDefinitions() []any {
	def := func(name, description string, props map[string]any, required ...string) any {
		return map[string]any{"type": "function", "function": map[string]any{"name": name, "description": description, "parameters": map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}}}
	}
	str := map[string]string{"type": "string"}
	return []any{
		def("read_file", "Read a UTF-8 workspace file (up to 64 KiB).", map[string]any{"path": str}, "path"),
		def("write_file", "Write a UTF-8 workspace file (up to 64 KiB).", map[string]any{"path": str, "content": str}, "path", "content"),
		def("list_files", "List a workspace directory.", map[string]any{"path": str}, "path"),
		def("run_command", "Run a foreground shell command in the workspace, maximum 30 seconds. Background processes are unsupported.", map[string]any{"command": str}, "command"),
		def("provider_request", "Call an approved named data operation through the credential broker. Requires an authorized case and justification.", map[string]any{"operation": str, "case_id": str, "justification": str, "parameters": map[string]string{"type": "object"}}, "operation", "case_id", "justification", "parameters"),
	}
}

func (m *Manager) tool(ctx context.Context, name string, raw json.RawMessage) string {
	switch name {
	case "read_file", "write_file", "list_files":
		return filetools.New(m.root).Call(ctx, name, raw)
	}

	var args struct {
		Command       string         `json:"command"`
		Operation     string         `json:"operation"`
		CaseID        string         `json:"case_id"`
		Justification string         `json:"justification"`
		Parameters    map[string]any `json:"parameters"`
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&args) != nil {
		return `{"error":"INVALID_TOOL_ARGUMENTS"}`
	}
	switch name {
	case "provider_request":
		if !shared.ValidID(args.Operation) || m.cfg.BrokerURL == "" {
			return `{"error":"INVALID_OPERATION"}`
		}
		b, e := json.Marshal(map[string]any{"case_id": args.CaseID, "justification": args.Justification, "parameters": args.Parameters})
		if e != nil || len(b) > 64<<10 {
			return `{"error":"INVALID_OPERATION"}`
		}
		requestCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
		defer cancel()
		req, e := http.NewRequestWithContext(requestCtx, "POST", strings.TrimRight(m.cfg.BrokerURL, "/")+"/v1/operations/"+args.Operation, bytes.NewReader(b))
		if e != nil {
			return `{"error":"PROVIDER_UNAVAILABLE"}`
		}
		req.Header.Set("Authorization", "Bearer "+m.cfg.Token)
		req.Header.Set("Content-Type", "application/json")
		resp, e := m.cfg.HTTP.Do(req)
		if e != nil {
			return `{"error":"PROVIDER_UNAVAILABLE"}`
		}
		defer resp.Body.Close()
		out, e := io.ReadAll(io.LimitReader(resp.Body, 65537))
		if e != nil || len(out) > 65536 || resp.StatusCode != 200 || !json.Valid(out) {
			return `{"error":"PROVIDER_REQUEST_REJECTED"}`
		}
		return string(out)
	case "run_command":
		if len(args.Command) == 0 || len(args.Command) > 8192 {
			return `{"error":"INVALID_COMMAND"}`
		}
		commandCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		executable, err := os.Executable()
		if err != nil {
			return `{"error":"COMMAND_UNAVAILABLE"}`
		}
		cmd := exec.CommandContext(commandCtx, executable, commandSupervisorArg, args.Command)
		cmd.Dir = m.cfg.DataDir + "/files"
		// Do not inherit grant tokens, service addresses or host process environment.
		cmd.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=" + cmd.Dir, "LANG=C.UTF-8"}
		if m.cfg.EgressURL != "" && m.cfg.ConnectionMode != "open" {
			cmd.Env = egress.Environment(cmd.Env, m.cfg.InferenceURL, m.cfg.BrokerURL)
		}
		out := shared.NewCappedOutput(64 << 10)
		cmd.Stdout = out
		cmd.Stderr = out
		cmd.WaitDelay = 3 * time.Second
		cmd.Cancel = func() error {
			if cmd.Process != nil {
				return cmd.Process.Signal(syscall.SIGTERM)
			}
			return nil
		}
		e := cmd.Start()
		if e != nil {
			return `{"error":"COMMAND_UNAVAILABLE"}`
		}
		e = cmd.Wait()
		code := 0
		if e != nil {
			code = -1
			if ee, ok := e.(*exec.ExitError); ok {
				code = ee.ExitCode()
			}
		}
		b, _ := json.Marshal(map[string]any{"exit_code": code, "output": string(out.Data), "timed_out": commandCtx.Err() != nil})
		return string(b)
	default:
		return `{"error":"UNKNOWN_TOOL"}`
	}
}
