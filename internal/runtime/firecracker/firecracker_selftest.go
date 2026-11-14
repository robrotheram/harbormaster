package firecracker

import (
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"time"
)

// This is a real KVM/jailer acceptance check, separate from unit tests. It uses
// a disposable synthetic workspace and makes no provider or Internet requests.
func FirecrackerSelfTest(f *Firecracker, token, service string) error {
	id := "sandbox_check_" + shared.NewID("")
	call := func(method, path, authority, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+authority)
		w := httptest.NewRecorder()
		f.Handler().ServeHTTP(w, r)
		return w
	}
	w := call("POST", "/v1/workspaces/"+id+"/start", token, `{"agent_kind":"codex","connection_mode":"blocked"}`)
	if w.Code != 200 {
		if b, e := os.ReadFile(fcState + "/" + id + "/console.log"); e == nil {
			s := string(b)
			if len(s) > 12000 {
				s = s[len(s)-12000:]
			}
			fmt.Fprintln(os.Stderr, s)
		}
		return fmt.Errorf("boot refused: status %d %s", w.Code, w.Body.String())
	}
	var o shared.Observation
	if json.Unmarshal(w.Body.Bytes(), &o) != nil || !o.Running || !o.Ready || o.RuntimeType != "firecracker" || o.NetworkStatus != "no-nic-vsock" {
		return errors.New("boot evidence incomplete")
	}
	f.mu.Lock()
	v := f.vms[id]
	f.mu.Unlock()
	defer func() { _ = f.kill(v) }()
	b, e := os.ReadFile(fcState + "/" + id + "/console.log")
	if e != nil || !strings.Contains(string(b), "SANDBOX_NETWORK_CHECK: no NIC") {
		return errors.New("guest bypass checks did not complete")
	}
	grant := f.grant(v)
	if f.hermes {
		status, b, err := f.agent(context.Background(), v, "GET", "/v1/runtime-capabilities", nil)
		var c struct {
			Hermes bool `json:"hermes"`
		}
		if err != nil || status != 200 || json.Unmarshal(b, &c) != nil || !c.Hermes {
			return fmt.Errorf("Hermes engine import failed inside the VM: status %d, response %s, transport %v", status, b, err)
		}
	}
	// A healthy guest may still be an older image that rejects the work
	// dispatch protocol. Probe digest validation without accepting a task or
	// contacting a provider, before admitting this image for live work.
	w = call("POST", "/v1/workspaces/"+id+"/agent/v1/tasks/from-file", token,
		`{"instruction_path":"acceptance-probe.md","specification_digest":"invalid","idempotency_key":"acceptance-protocol-probe"}`)
	var acceptance shared.APIError
	if w.Code != 400 || json.Unmarshal(w.Body.Bytes(), &acceptance) != nil || acceptance.Code != "INVALID_SPECIFICATION_DIGEST" {
		return fmt.Errorf("guest work specification protocol unavailable: status %d; rebuild and stage the workspace guest image", w.Code)
	}
	// Exercise a real guest PTY before installing this runtime for operators.
	terminalPath := "/v1/workspaces/" + id + "/agent/v1/terminals"
	w = call("POST", terminalPath, token, `{"id":"terminal_acceptance","cols":80,"rows":24}`)
	if w.Code != 201 {
		return fmt.Errorf("guest terminal unavailable: status %d; rebuild the guest image", w.Code)
	}
	terminalPath += "/terminal_acceptance"
	w = call("POST", terminalPath+"/exchange", token, `{"input":"printf 'terminal-%s\\n' ready; exit\r","offset":0,"cols":100,"rows":30}`)
	terminalReady := false
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		var result struct {
			Output []byte `json:"output"`
			Exited bool   `json:"exited"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil {
			return fmt.Errorf("guest terminal exchange failed: status %d", w.Code)
		}
		if result.Exited && strings.Contains(string(result.Output), "terminal-ready") {
			terminalReady = true
			break
		}
		time.Sleep(25 * time.Millisecond)
		w = call("POST", terminalPath+"/exchange", token, `{"offset":0,"cols":100,"rows":30}`)
	}
	w = call("POST", terminalPath+"/close", token, `{}`)
	if !terminalReady || w.Code != 200 {
		return errors.New("guest terminal command or cleanup failed")
	}
	grantBody := func(a string) string { return `{"token":"` + grant + `","audience":"` + a + `"}` }
	if w := call("POST", "/v1/grants/verify", service, grantBody("inference")); w.Code != 200 {
		return errors.New("running generation grant rejected")
	}
	if w := call("POST", "/v1/grants/verify", service, grantBody("egress")); w.Code != 403 {
		return errors.New("blocked egress grant accepted")
	}
	if w := call("POST", "/v1/workspaces/"+id+"/network-policy", token, `{"connections":{"mode":"open","destinations":[]}}`); w.Code != 403 {
		return errors.New("open networking accepted")
	}
	if w := call("POST", "/v1/workspaces/"+id+"/start", token, `{"project_id":"foreign","home_id":"foreign"}`); w.Code == 200 {
		return errors.New("project ownership reassignment accepted")
	}
	if w := call("POST", "/v1/workspaces/"+id+"/stop", token, ""); w.Code != 200 {
		return fmt.Errorf("sandbox did not stop cleanly: %d %s (activity: %+v)", w.Code, w.Body.String(), o.Activity)
	}
	if w := call("POST", "/v1/grants/verify", service, grantBody("inference")); w.Code != 403 {
		return errors.New("stopped grant remained usable")
	}
	return nil
}
