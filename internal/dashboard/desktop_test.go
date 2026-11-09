package dashboard

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"agent-workbench/internal/shared"
)

func TestDesktopUsesClientAuthorityAndRejectsForeignOrigin(t *testing.T) {
	var calls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer control" || r.Header.Get(shared.NativeAuthorizationHeader) != "DPoP native-client" || r.Header.Get("Cookie") != "" || r.URL.Path != "/v1/workspaces/ws_0123456789abcdef0123456789abcdef/desktop/api/auth/session" {
			t.Error("desktop authority was not separated")
		}
		io.WriteString(w, `{"authenticated":true}`)
	}))
	defer upstream.Close()
	h, err := NewInteractiveHandler(upstream.URL, upstream.URL, "control")
	if err != nil {
		t.Fatal(err)
	}
	for _, origin := range []string{"t3code://app", "https://evil.example"} {
		r := httptest.NewRequest("GET", "/api/auth/session", nil)
		r.Host = "desktop-ws-0123456789abcdef0123456789abcdef.localhost:8080"
		r.Header.Set("Origin", origin)
		r.Header.Set("Authorization", "DPoP native-client")
		r.Header.Set(shared.NativeAuthorizationHeader, "forged-control-token")
		r.Header.Set("Cookie", "awb_console=browser-cookie")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if origin == "t3code://app" && w.Code != 200 {
			t.Fatal("native desktop rejected")
		}
		if origin != "t3code://app" && w.Code != 403 {
			t.Fatal("foreign origin accepted")
		}
	}
	if calls != 1 {
		t.Fatal("rejected origin reached upstream")
	}
}
