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
