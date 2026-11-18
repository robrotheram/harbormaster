package firecracker

import (
	"agent-workbench/internal/runtime/protocol"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
)

func TestFirecrackerReadinessReportsLastFailure(t *testing.T) {
	verificationErr := errors.New("VMM identity unavailable")
	resumeErr := errors.New("guest connection refused")
	for _, tt := range []struct {
		name      string
		verifyErr error
		status    int
		resumeErr error
		want      string
	}{
		{name: "VMM verification", verifyErr: verificationErr, want: verificationErr.Error()},
		{name: "guest transport", resumeErr: resumeErr, want: resumeErr.Error()},
		{name: "guest response", status: http.StatusServiceUnavailable, want: "guest resume returned 503"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			err := waitForFirecrackerResume(ctx, func() error {
				if tt.verifyErr != nil {
					cancel()
				}
				return tt.verifyErr
			}, func() (int, error) {
				if tt.verifyErr != nil {
					t.Fatal("contacted guest before verifying the VMM")
				}
				cancel()
				return tt.status, tt.resumeErr
			})
			want := "context canceled; last readiness check: " + tt.want
			if !errors.Is(err, context.Canceled) || err.Error() != want {
				t.Fatalf("readiness error = %v, want %q", err, want)
			}
		})
	}
}

func TestFirecrackerReadinessTimeoutWrapsResumeError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		resumeErr := errors.New("guest connection refused")
		err := waitForFirecrackerResume(context.Background(), func() error {
			return nil
		}, func() (int, error) {
			return 0, resumeErr
		})
		if !errors.Is(err, resumeErr) || err.Error() != "sandbox did not become ready: guest connection refused" {
			t.Fatalf("readiness timeout = %v, want wrapped resume error", err)
		}
	})
}

func TestFirecrackerConsoleBoundDoesNotInterruptVMM(t *testing.T) {
	var buf bytes.Buffer
	c := &fcConsole{file: &buf, remaining: 5}
	for _, s := range []string{"abc", "defgh", "more"} {
		n, e := c.Write([]byte(s))
		if e != nil || n != len(s) {
			t.Fatal("console bound interrupted writer", n, e)
		}
	}
	if buf.String() != "abcde" || c.remaining != 0 {
		t.Fatal("console exceeded storage bound", buf.String(), c.remaining)
	}
}

func testFC() *Firecracker {
	return &Firecracker{token: strings.Repeat("c", 32), service: strings.Repeat("g", 32), secret: strings.Repeat("s", 32), vms: map[string]*fcVM{}}
}
func fcRequest(f *Firecracker, method, path, token, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	f.Handler().ServeHTTP(w, r)
	return w
}
func TestFirecrackerRefusesOpenBeforeProvisioning(t *testing.T) {
	f := testFC()
	_, err := f.launch(context.Background(), "test", protocol.WorkspaceSpec{ConnectionMode: "open"})
	if err == nil {
		t.Fatal("unrestricted networking must never reach provisioning")
	}
	f.vms["test"] = &fcVM{ID: "test", running: true}
	w := fcRequest(f, "POST", "/v1/workspaces/test/network-policy", f.token, `{"connections":{"mode":"open","destinations":[]}}`)
	if w.Code != 403 {
		t.Fatal(w.Code, w.Body.String())
	}
	if f.vms["test"].Policy.Mode == "open" {
		t.Fatal("open policy applied")
	}
}
func TestFirecrackerManagementAndGrantAuthoritySeparate(t *testing.T) {
	f := testFC()
	for _, q := range []struct{ method, path, token, body string }{
		{"GET", "/v1/capabilities", f.service, ""},
		{"POST", "/v1/grants/verify", f.token, `{"token":"x","audience":"egress"}`},
		{"POST", "/v1/workspaces/test/start", "", "{}"},
	} {
		if got := fcRequest(f, q.method, q.path, q.token, q.body); got.Code == 200 {
			t.Fatal("cross-authority request accepted", q.path)
		}
	}
	w := fcRequest(f, "POST", "/v1/grants/verify", f.service, `{"token":"`+strings.Repeat("x", 64)+`","audience":"egress"}`)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}
func TestFirecrackerMissingHostNeverFallsBack(t *testing.T) {
	t.Setenv("FIRECRACKER_SOCKET", t.TempDir()+"/missing.sock")
	if ServeFirecrackerBridge() == nil {
		t.Fatal("bridge started without host controller")
	}
}
func TestFirecrackerRejectsOwnershipChange(t *testing.T) {
	a := protocol.WorkspaceSpec{ProjectID: "one", HomeID: "home", AgentKind: "codex", TemplateID: protocol.DevelopmentTemplate}
	b := a
	b.ProjectID = "two"
	if checkFCSpec(a, b) == nil {
		t.Fatal("project reassignment accepted")
	}
	b = a
	b.HomeID = "other"
	if checkFCSpec(a, b) == nil {
		t.Fatal("home reassignment accepted")
	}
}
func TestFirecrackerDevicesExcludeEscapeRoutes(t *testing.T) {
	b, e := json.Marshal(fcConfig(1024))
	if e != nil {
		t.Fatal(e)
	}
	var cfg map[string]json.RawMessage
	if json.Unmarshal(b, &cfg) != nil {
		t.Fatal("bad config")
	}
	for _, key := range []string{"network-interfaces", "mmds-config", "balloon", "vhost-user"} {
		if _, ok := cfg[key]; ok {
			t.Fatal("unexpected device", key)
		}
	}
	var disks []struct {
		ID string `json:"drive_id"`
		RO bool   `json:"is_read_only"`
	}
	json.Unmarshal(cfg["drives"], &disks)
	if len(disks) != 3 {
		t.Fatal("unexpected host disk exposure")
	}
	for _, d := range disks {
		if d.ID != "data" && !d.RO {
			t.Fatal("writable system/config disk")
		}
	}
}
func TestFirecrackerStoppedWorkspaceGrantRevoked(t *testing.T) {
	f := testFC()
	v := &fcVM{ID: "test", Generation: "one", running: false}
	f.vms[v.ID] = v
	w := fcRequest(f, "POST", "/v1/grants/verify", f.service, `{"token":"`+f.grant(v)+`","audience":"inference"}`)
	if w.Code != http.StatusForbidden {
		t.Fatal(w.Code)
	}
	v2 := &fcVM{ID: v.ID, Generation: "two"}
	if f.grant(v) == f.grant(v2) {
		t.Fatal("grant survived generation change")
	}
}
