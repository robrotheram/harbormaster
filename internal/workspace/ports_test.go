package workspace

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPortDiscoveryForwardAndListenerLifecycle(t *testing.T) {
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			w.Header().Set("Location", "http://"+r.Host+"/result?x=1")
			w.WriteHeader(302)
			return
		}
		fmt.Fprint(w, "detected app")
	}))
	defer app.Close()
	_, p, _ := net.SplitHostPort(strings.TrimPrefix(app.URL, "http://"))
	m, err := New(Config{ID: "ws_ports", DataDir: t.TempDir(), Token: "test-workspace-token"})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	res := invoke(t, m.Handler(), "GET", "/v1/ports", "")
	var found struct {
		Items []detectedPort `json:"items"`
	}
	json.Unmarshal(res.Body.Bytes(), &found)
	var chosen detectedPort
	for _, port := range found.Items {
		if fmt.Sprint(port.Port) == p {
			chosen = port
		}
	}
	if chosen.Listener == "" {
		t.Fatal("HTTP listener not detected", res.Body.String())
	}
	body := fmt.Sprintf(`{"application":{"kind":"forward","directory":".","command":"","port":%d,"listener":%q},"ttl_seconds":3600,"idempotency_key":"forward-test"}`, chosen.Port, chosen.Listener)
	created := invoke(t, m.Handler(), "POST", "/v1/services", body)
	if created.Code != 201 {
		t.Fatal(created.Code, created.Body.String())
	}
	var view applicationView
	json.Unmarshal(created.Body.Bytes(), &view)
	if !view.Ready {
		t.Fatal("forward not ready")
	}
	if r := invoke(t, m.Handler(), "POST", "/v1/services", body); r.Code != 200 {
		t.Fatal("retry not idempotent", r.Code)
	}
	path := "/v1/application-proxy/" + view.ID + "/"
	if r := invoke(t, m.Handler(), "GET", path, ""); r.Code != 200 || r.Body.String() != "detected app" {
		t.Fatal("forward failed", r.Code, r.Body.String())
	}
	if r := invoke(t, m.Handler(), "GET", path+"redirect", ""); r.Code != 302 || r.Header().Get("Location") != "/result?x=1" {
		t.Fatal("redirect escaped preview", r.Code, r.Header())
	}
	app.Close()
	// A new process on the same port must never inherit the old preview grant.
	listener, err := net.Listen("tcp", "127.0.0.1:"+p)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if r := invoke(t, m.Handler(), "GET", path, ""); r.Code != 410 {
		t.Fatal("grant followed a replaced listener", r.Code)
	}
	if r := invoke(t, m.Handler(), "POST", "/v1/services", strings.Replace(body, "forward-test", "different-key", 1)); r.Code != 409 {
		t.Fatal("stale listener admitted", r.Code)
	}
}
func TestDiscoveryExcludesManagementAndNonlocalAddresses(t *testing.T) {
	row := func(addr string, port int) string {
		return fmt.Sprintf("0: %s:%04X 00000000:0000 0A 0 0 0 10001 0 12345\n", addr, port)
	}
	text := row("0100007F", 3000) + row("00000000", 5173) + row("0100007F", 3773) + row("0100007F", 8080) + row("0100007F", 8099) + row("0100007F", 18085) + row("0200000A", 4000)
	ports := parseListeningPorts(text, false)
	if len(ports) != 2 || ports[0].Port != 3000 || ports[1].Port != 5173 {
		t.Fatal(ports)
	}
	if got := parseListeningPorts(row("00000000000000000000000001000000", 3000), true); len(got) != 1 || got[0].Address != "[::1]:3000" {
		t.Fatal(got)
	}
}
