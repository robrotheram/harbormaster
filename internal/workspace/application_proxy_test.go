package workspace

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestApplicationProxyStreamsUpgradeAndStripsAuthority(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/socket" || r.URL.RawQuery != "v=1" {
			t.Error("request path lost")
		}
		if r.Header.Get("Authorization") != "" || strings.Contains(r.Header.Get("Cookie"), "awb_") {
			t.Error("management credential leaked")
		}
		if r.Header.Get("Cookie") != "app=hello" {
			t.Error("application cookie lost")
		}
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n")
		rw.Flush()
		io.Copy(conn, conn)
	}))
	defer upstream.Close()
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(upstream.URL, "http://"))
	m, err := New(Config{ID: "ws_app", HomeID: "home_test", DataDir: t.TempDir(), Token: "test-workspace-token"})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	m.db.Exec(`INSERT INTO retained_services VALUES('service_app','active',?,'{}')`, time.Now().Unix()+60)
	m.db.Exec(`INSERT INTO workspace_applications VALUES('service_app','test','test',?)`, fmt.Sprintf(`{"directory":".","command":"owned-test-server","port":%s}`, port))
	server := httptest.NewServer(m.Handler())
	defer server.Close()
	conn, err := net.Dial("tcp", strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	fmt.Fprintf(conn, "GET /v1/application-proxy/service_app/socket?v=1 HTTP/1.1\r\nHost: workspace\r\nAuthorization: Bearer test-workspace-token\r\nCookie: awb_preview=private; app=hello\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nOrigin: http://preview.localhost\r\n\r\n")
	reader := bufio.NewReader(conn)
	res, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 101 {
		t.Fatal(res.Status)
	}
	conn.Write([]byte("ping"))
	data := make([]byte, 4)
	if _, err = io.ReadFull(reader, data); err != nil || string(data) != "ping" {
		t.Fatal("upgrade was not bidirectional", err)
	}
	conn.Close()
	invoke(t, m.Handler(), "POST", "/v1/services/service_app/revoke", `{}`)
	if res := invoke(t, m.Handler(), "GET", "/v1/application-proxy/service_app/socket", ""); res.Code != 410 {
		t.Fatal("revoked app reachable", res.Code)
	}
}
