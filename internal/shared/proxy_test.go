package shared

import (
	"crypto/sha1"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProxyPreservesWebSocketUpgradeAndBytes(t *testing.T) {
	native := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer private-native-session" || r.URL.Path != "/ws" || r.Host != "workspace.localhost" {
			t.Error("proxy routing/auth failed")
		}
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		digest := sha1.Sum([]byte(r.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
		io.WriteString(conn, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: "+base64.StdEncoding.EncodeToString(digest[:])+"\r\n\r\n")
		b := make([]byte, 4)
		if _, err = io.ReadFull(conn, b); err == nil {
			_, _ = conn.Write(b)
		}
	}))
	defer native.Close()
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { Proxy(w, r, native.URL, "/ws", "private-native-session") }))
	defer gateway.Close()
	req, _ := http.NewRequest("GET", gateway.URL+"/console/ws?orchestrationProtocol=2", nil)
	req.Host = "workspace.localhost"
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 101 {
		t.Fatalf("upgrade: %d", response.StatusCode)
	}
	stream, ok := response.Body.(io.ReadWriteCloser)
	if !ok {
		t.Fatal("upgraded connection unavailable")
	}
	if _, err = stream.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 4)
	if _, err = io.ReadFull(stream, b); err != nil || string(b) != "ping" {
		t.Fatal("duplex stream lost")
	}
}
