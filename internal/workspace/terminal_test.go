package workspace

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type terminalReply struct {
	Output    []byte `json:"output"`
	Offset    int64  `json:"offset"`
	Exited    bool   `json:"exited"`
	Truncated bool   `json:"truncated"`
}

func TestTerminalInteractiveLifecycle(t *testing.T) {
	t.Setenv("PRIVATE_MANAGEMENT_SECRET", "must-not-inherit")
	m, err := New(Config{ID: "ws_terminal", HomeID: "home_test", DataDir: t.TempDir(), Token: "test-workspace-token"})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	h := m.Handler()
	if r := invoke(t, h, "POST", "/v1/terminals", `{"id":"terminal_test","cols":80,"rows":24}`); r.Code != 201 {
		t.Fatal(r.Code, r.Body.String())
	}
	if r := invoke(t, h, "POST", "/v1/terminals", `{"id":"terminal_test","cols":80,"rows":24}`); r.Code != 200 {
		t.Fatal("duplicate terminal", r.Body.String())
	}
	if m.Activity().BackgroundJobs != 1 {
		t.Fatal("shell not counted as active")
	}
	if r := invoke(t, h, "POST", "/v1/quiesce", `{}`); r.Code == 200 {
		t.Fatal("quiesced an active shell")
	}
	offset := int64(0)
	exchange := func(input string) terminalReply {
		t.Helper()
		body, _ := json.Marshal(terminalInput{Input: input, Offset: offset, Cols: 100, Rows: 30})
		r := invoke(t, h, "POST", "/v1/terminals/terminal_test/exchange", string(body))
		if r.Code != 200 {
			t.Fatal(r.Code, r.Body.String())
		}
		var reply terminalReply
		if err := json.Unmarshal(r.Body.Bytes(), &reply); err != nil {
			t.Fatal(err)
		}
		offset = reply.Offset
		return reply
	}
	collect := func(input, want string) string {
		t.Helper()
		output := string(exchange(input).Output)
		deadline := time.Now().Add(4 * time.Second)
		for !strings.Contains(output, want) && time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
			output += string(exchange("").Output)
		}
		if !strings.Contains(output, want) {
			t.Fatalf("missing %q in %q", want, output)
		}
		return output
	}
	collect("stty -echo\r", "")
	output := collect("printf '\\nDIR='; pwd; stty size; printf 'secret=%s\\n' \"$PRIVATE_MANAGEMENT_SECRET\"; printf 'ready-%s\\n' '✓'\r", "ready-✓")
	if !strings.Contains(output, m.cfg.DataDir+"/files") || !strings.Contains(output, "30 100") || strings.Contains(output, "must-not-inherit") {
		t.Fatal(output)
	}
	exchange("sleep 30\r")
	time.Sleep(100 * time.Millisecond)
	exchange("\x03")
	collect("printf 'after-%s\\n' interrupt\r", "after-interrupt")
	reply := exchange("exit\r")
	for deadline := time.Now().Add(3 * time.Second); !reply.Exited && time.Now().Before(deadline); {
		time.Sleep(20 * time.Millisecond)
		reply = exchange("")
	}
	if !reply.Exited {
		t.Fatal("shell exit not reported")
	}
	if r := invoke(t, h, "POST", "/v1/terminals/terminal_test/close", `{}`); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	if m.Activity().BackgroundJobs != 0 {
		t.Fatal("closed terminal kept worker busy")
	}
}

func TestTerminalBoundsExpiryAndAuthentication(t *testing.T) {
	m, err := New(Config{ID: "ws_terminal", HomeID: "home_test", DataDir: t.TempDir(), Token: "test-workspace-token"})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	h := m.Handler()
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/terminals", strings.NewReader(`{"id":"bad","cols":80,"rows":24}`)))
	if r.Code != 401 {
		t.Fatal("unauthenticated terminal", r.Code)
	}
	if r := invoke(t, h, "POST", "/v1/terminals", `{"id":"bad","cols":501,"rows":24}`); r.Code != 400 {
		t.Fatal("unbounded dimensions")
	}
	for i := 0; i < 4; i++ {
		r := invoke(t, h, "POST", "/v1/terminals", fmt.Sprintf(`{"id":"terminal_%d","cols":80,"rows":24}`, i))
		if r.Code != 201 {
			t.Fatal(r.Body.String())
		}
	}
	if r := invoke(t, h, "POST", "/v1/terminals", `{"id":"terminal_fifth","cols":80,"rows":24}`); r.Code != 429 {
		t.Fatal("unbounded sessions")
	}
	m.mu.Lock()
	s := m.terminals["terminal_0"]
	m.mu.Unlock()
	s.mu.Lock()
	s.lastSeen = time.Now().Add(-2 * terminalLease)
	s.mu.Unlock()
	select {
	case <-s.done:
	case <-time.After(7 * time.Second):
		t.Fatal("orphan terminal did not expire")
	}
	if r := invoke(t, h, "POST", "/v1/terminals/terminal_0/exchange", `{"offset":0,"cols":80,"rows":24}`); r.Code != 404 {
		t.Fatal("expired terminal still accessible", r.Body.String())
	}
}

func TestTerminalCloseKillsJobsAndBoundsOutput(t *testing.T) {
	m, err := New(Config{ID: "ws_terminal", HomeID: "home_test", DataDir: t.TempDir(), Token: "test-workspace-token"})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	h := m.Handler()
	if r := invoke(t, h, "POST", "/v1/terminals", `{"id":"terminal_test","cols":80,"rows":24}`); r.Code != 201 {
		t.Fatal(r.Body.String())
	}
	m.mu.Lock()
	s := m.terminals["terminal_test"]
	m.mu.Unlock()
	if err := s.tty.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal("PTY does not support write deadlines", err)
	}
	send := func(input string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(terminalInput{Input: input, Cols: 80, Rows: 24})
		return invoke(t, h, "POST", "/v1/terminals/terminal_test/exchange", string(body))
	}
	if r := send(strings.Repeat("x", 4097)); r.Code != 400 {
		t.Fatal("unbounded input accepted")
	}
	if r := send("head -c 300000 /dev/zero | tr '\\0' x\r"); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	for deadline := time.Now().Add(3 * time.Second); ; {
		s.mu.Lock()
		size := len(s.output.data)
		truncated := s.output.end > int64(size)
		s.mu.Unlock()
		if truncated {
			if size > terminalBufferLimit {
				t.Fatal("output buffer unbounded")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("output did not arrive")
		}
		time.Sleep(20 * time.Millisecond)
	}
	var reply terminalReply
	r := send("")
	if json.Unmarshal(r.Body.Bytes(), &reply) != nil || !reply.Truncated || len(reply.Output) > terminalBufferLimit {
		t.Fatal(r.Body.String())
	}
	send("sleep 30 & echo $! > debug.pid; wait\r")
	var pid int
	for deadline := time.Now().Add(3 * time.Second); pid == 0; {
		data, _ := os.ReadFile(filepath.Join(m.cfg.DataDir, "files", "debug.pid"))
		pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
		if time.Now().After(deadline) {
			t.Fatal("shell job did not start")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if r := invoke(t, h, "POST", "/v1/terminals/terminal_test/close", `{}`); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
		t.Fatal("shell did not close")
	}
	for deadline := time.Now().Add(2 * time.Second); ; {
		stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if os.IsNotExist(err) || processState(string(stat)) == "Z" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("terminal job survived close")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestTerminalOutputWrapsAndKeepsOffsets(t *testing.T) {
	var output terminalOutput
	var history []byte
	for _, chunk := range [][]byte{
		[]byte("first"),
		bytes.Repeat([]byte("12"), terminalBufferLimit/2),
		bytes.Repeat([]byte("345"), terminalBufferLimit/2),
		[]byte("last"),
	} {
		output.append(chunk)
		history = append(history, chunk...)
		end := int64(len(history))
		base := max(0, end-terminalBufferLimit)
		if output.end != end || int64(len(output.data)) != end-base {
			t.Fatalf("buffer ends at %d and retains %d bytes; want %d and %d", output.end, len(output.data), end, end-base)
		}
		for _, offset := range []int64{0, base, (base + end) / 2, end} {
			got, truncated := output.read(offset)
			if !bytes.Equal(got, history[max(offset, base):]) || truncated != (offset < base) {
				t.Fatalf("incorrect output at byte %d of %d (truncated=%v)", offset, end, truncated)
			}
		}
	}
	chunk := make([]byte, 8192)
	if got := testing.AllocsPerRun(64, func() { output.append(chunk) }); got != 0 {
		t.Fatalf("full scrollback allocated %v times per append", got)
	}
}
