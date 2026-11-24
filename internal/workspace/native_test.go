package workspace

import (
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNativeRunnerUsesOnlyWorkspaceGrant(t *testing.T) {
	directory := t.TempDir()
	for _, provider := range []string{"codex", "claude"} {
		// The fake CLIs assert the native stdin, work directory and environment. No
		// upstream call is made; live interoperability is a separate integration check.
		script := `#!/bin/sh
test -z "$OPENAI_API_KEY$ANTHROPIC_API_KEY$VAULT_TOKEN$WORKSPACE_TOKEN" || exit 11
input=$(cat)
case "$input" in
  *'If this task delivers or changes a runnable web app'*'workbench-preview start'*'check that workbench-preview status'*'workbench-analysis register-artifact'*'harbormaster-visualization'*'implement sample') ;;
  *'Desktop app development'*'workbench-preview desktop'*'workbench-desktop run'*'implement a GUI')
    test "$XDG_SESSION_TYPE" = wayland && test "$WAYLAND_DISPLAY" = wayland-0 && test -n "$XDG_RUNTIME_DIR" || exit 17
    case "$input" in *'workbench-preview start'*) exit 18;; esac ;;
  *) exit 12;;
esac
test "$WORKBENCH_WORKSPACE_GRANT" = workspace-only-grant || exit 15
case "$*" in *selected-model*) ;; *) exit 16;; esac
printf 'native file\n' > native.txt
if [ -n "$WORKBENCH_INFERENCE_GRANT" ]; then
  test "$WORKBENCH_INFERENCE_GRANT" = workspace-only-grant || exit 13
  while [ "$#" -gt 0 ]; do if [ "$1" = '-o' ]; then shift; printf 'codex result' > "$1"; break; fi; shift; done
else
  test "$CLAUDE_CODE_OAUTH_TOKEN" = workspace-only-grant || exit 14
  printf '{"result":"claude result","is_error":false}'
fi
`
		if err := os.WriteFile(filepath.Join(directory, provider), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", directory+":/usr/bin:/bin")
	t.Setenv("OPENAI_API_KEY", "must-not-inherit")
	t.Setenv("VAULT_TOKEN", "must-not-inherit")
	m, err := New(Config{ID: "ws_native", HomeID: "home_test", Token: "workspace-only-grant", DataDir: t.TempDir(), InferenceURL: "http://inference-gateway:8080", CodexModel: "approved-codex", ClaudeModel: "approved-claude"})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	for _, provider := range []string{"codex", "claude"} {
		result, code := m.runNativeTask(context.Background(), provider, "implement sample", "", "selected-model", "high", "")
		if code != "" || result != provider+" result" {
			t.Fatalf("%s runner: %s %q", provider, code, result)
		}
	}
	if b, err := os.ReadFile(filepath.Join(m.cfg.DataDir, "files/native.txt")); err != nil || !strings.Contains(string(b), "native file") {
		t.Fatal("agent did not execute in workspace")
	}
	first, status, err := m.submit(submit{Instruction: "implement sample", Key: "native-task", Model: "selected-model", Provider: "codex"})
	if err != nil || status != 202 {
		t.Fatal("submit failed")
	}
	done := waitTask(t, m, first.ID)
	if done.State != "succeeded" {
		t.Fatalf("native task %+v", done)
	}
	duplicate, _, err := m.submit(submit{Instruction: "implement sample", Key: "native-task", Model: "selected-model", Provider: "codex"})
	if err != nil || duplicate.ID != first.ID {
		t.Fatal("native dedup failed")
	}
	if _, status, err := m.submit(submit{Instruction: "implement sample", Key: "native-task", Model: "selected-model", Provider: "claude"}); err == nil || status != 409 {
		t.Fatal("provider change did not conflict")
	}
	if err := os.WriteFile(filepath.Join(m.cfg.DataDir, "files/gui.md"), []byte("implement a GUI"), 0600); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/v1/tasks/from-file", strings.NewReader(`{"instruction_path":"gui.md","idempotency_key":"gui","provider":"codex","model":"selected-model","work_kind":"`+shared.WorkKindDesktop+`"}`))
	request.Header.Set("Authorization", "Bearer "+m.cfg.Token)
	response := httptest.NewRecorder()
	m.Handler().ServeHTTP(response, request)
	if response.Code != 202 {
		t.Fatalf("desktop task rejected: %d %s", response.Code, response.Body)
	}
	var task shared.Task
	if err := json.Unmarshal(response.Body.Bytes(), &task); err != nil {
		t.Fatal(err)
	}
	if done := waitTask(t, m, task.ID); done.State != "succeeded" {
		t.Fatalf("desktop task %+v", done)
	}
}

func TestNativeCancellationKillsProcess(t *testing.T) {
	directory := t.TempDir()
	os.WriteFile(filepath.Join(directory, "claude"), []byte("#!/bin/sh\ncat >/dev/null\nsleep 30\n"), 0700)
	t.Setenv("PATH", directory+":/usr/bin:/bin")
	m, err := New(Config{ID: "ws_cancel_native", HomeID: "home_test", Token: "grant", DataDir: t.TempDir(), ClaudeModel: "model"})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, code := m.runNativeTask(ctx, "claude", "wait", "", "", "", ""); code != "EXECUTION_INTERRUPTED" {
		t.Fatal("native cancellation failed")
	}
}

func TestProcessStateHandlesNamesAndZombies(t *testing.T) {
	for _, tt := range []struct{ stat, want string }{{"45 (git) Z 1 0 0", "Z"}, {"7 (a name (with parentheses)) S 1 0", "S"}, {"garbled", ""}, {"7 (a)   ", ""}} {
		if got := processState(tt.stat); got != tt.want {
			t.Fatalf("%q: %q", tt.stat, got)
		}
	}
}

func TestDataScienceInstructionUsesArtifactDelivery(t *testing.T) {
	got := dataScienceInstruction("Find anomalies.")
	for _, want := range []string{"Find anomalies.", "workbench-analysis register-artifact", "Do not build a website"} {
		if !strings.Contains(got, want) {
			t.Fatalf("data science prompt is missing %q", want)
		}
	}
	if strings.Contains(got, workspacePreviewGuidance) {
		t.Fatal("data science prompt asks for a web preview")
	}
}
