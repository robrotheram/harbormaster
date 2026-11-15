package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSubscriptionIdentityCannotBeSpoofed(t *testing.T) {
	var received string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = r.Header.Get("X-Workbench-Subscription-User")
		w.Write([]byte(`{"saved":false}`))
	}))
	defer up.Close()
	h, err := NewInteractiveHandler(up.URL, up.URL, "control")
	if err != nil {
		t.Fatal(err)
	}
	for _, verified := range []bool{false, true} {
		r := httptest.NewRequest("GET", "http://localhost/ui-api/subscription", nil)
		r.Header.Set("X-Workbench-Subscription-User", "user_victim")
		if verified {
			r = withSubscriptionIdentity(r, "issuer", "alice")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
		if received == "user_victim" || (!verified && received != "") || (verified && !strings.HasPrefix(received, "user_")) {
			t.Fatal("unverified identity forwarded")
		}
	}
}
