// Package nativeagent adapts pinned native coding agents to supervised tasks.
// It owns invocation and result protocols, with no workspace database or HTTP server.
package nativeagent

import (
	"agent-workbench/internal/shared"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

type MCPTransport string

const KubernetesGeneration MCPTransport = "kubernetes-generation"
const KubernetesMCPURL = "http://hermes-mcp.harbormaster.svc:8080/hermes-mcp"

// OutputLimit is the byte cap for native command responses.
const OutputLimit = 1 << 20

type Config struct {
	HermesMCPTransport MCPTransport
	DesktopEnv         []string
	ReasoningEffort    string
	ToolPath           string

	DataDir, HomeDir, AgentKind, Token, InferenceURL, ProjectID, WorkspaceID, TaskID string
	CodexModel, ClaudeModel, HermesModel                                             string
	EgressURL, ConnectionMode                                                        string
}
type invocation struct {
	executable string
	args, env  []string
	resultFile string
}

func Run(ctx context.Context, cfg Config, provider, instruction string, stdoutEvents, stderrEvents io.Writer) (string, string) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	plan, code := prepare(cfg, provider)
	if code != "" {
		return "", code
	}
	if plan.resultFile != "" {
		defer os.Remove(plan.resultFile)
	}
	if stdoutEvents == nil {
		stdoutEvents = io.Discard
	}
	if stderrEvents == nil {
		stderrEvents = io.Discard
	}
	cmd := exec.CommandContext(ctx, plan.executable, plan.args...)
	cmd.Dir = filepath.Join(cfg.DataDir, "files")
	cmd.Env = plan.env
	cmd.Stdin = strings.NewReader(instruction)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = time.Second
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
	stdout := shared.NewCappedOutput(OutputLimit)
	cmd.Stdout = io.MultiWriter(stdout, stdoutEvents)
	stderr := shared.NewCappedOutput(OutputLimit)
	cmd.Stderr = io.MultiWriter(stderr, stderrEvents)
	if err := cmd.Start(); err != nil {
		return "", "AGENT_EXECUTABLE_UNAVAILABLE"
	}
	err := cmd.Wait()
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if ctx.Err() != nil {
		return "", "EXECUTION_INTERRUPTED"
	}
	if err != nil {
		return "", nativeFailureCode(stdout.Data, stderr.Data)
	}
	if provider == "codex" {
		file, err := os.Open(plan.resultFile)
		if err != nil {
			return "", "RESULT_UNAVAILABLE"
		}
		defer file.Close()
		b, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
		if err != nil || len(b) > 1<<20 {
			return "", "RESULT_TOO_LARGE"
		}
		return string(b), ""
	}
	var out struct {
		Result  string `json:"result"`
		IsError bool   `json:"is_error"`
	}
	if stdout.Overflow || json.Unmarshal(stdout.Data, &out) != nil {
		return "", "INVALID_AGENT_RESPONSE"
	}
	if out.IsError {
		return "", "NATIVE_AGENT_FAILED"
	}
	return out.Result, ""
}

func tomlString(s string) string { b, _ := json.Marshal(s); return string(b) }

// Only gateway codes cross the task boundary. Native CLI errors can contain
// prompts, credentials, or provider bodies and must never be returned wholesale.
var gatewayFailure = regexp.MustCompile(`"code"\s*:\s*"(PROVIDER_LOGIN_REQUIRED|MODEL_NOT_ALLOWED|CREDENTIAL_UNAVAILABLE|PROVIDER_NOT_CONFIGURED)"`)

func nativeFailureCode(output ...[]byte) string {
	for _, stream := range output {
		stream = bytes.ReplaceAll(stream, []byte(`\"`), []byte(`"`))
		if match := gatewayFailure.FindSubmatch(stream); match != nil {
			return string(match[1])
		}
	}
	return "NATIVE_AGENT_FAILED"
}
