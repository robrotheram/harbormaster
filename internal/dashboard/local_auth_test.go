package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLocalDashboardRequiresLauncherPassword(t *testing.T) {
	password := strings.Repeat("x", 32)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Fatal("local dashboard password forwarded")
		}
		w.WriteHeader(http.StatusNoContent)
	})
	handler, err := NewLocalAuthHandler(next, password)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		host, password string
		status         int
	}{
		{"localhost", "", 401},
		{"localhost", "wrong", 401},
		{"localhost", password, 204},
		{"remote.example", password, 403},
	} {
		r := httptest.NewRequest("GET", "http://"+test.host+"/ui-api/projects", nil)
		r.SetBasicAuth("operator", test.password)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != test.status {
			t.Errorf("%s: status %d, want %d", test.host, w.Code, test.status)
		}
	}
}
