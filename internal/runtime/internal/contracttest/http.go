package contracttest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type HTTPCase struct {
	Name, Method, Path, Token, Body string
	Status                          int
	Code                            string
}

func HTTP(t *testing.T, handler http.Handler, cases ...HTTPCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			r := httptest.NewRequest(tc.Method, tc.Path, strings.NewReader(tc.Body))
			if tc.Token != "" {
				r.Header.Set("Authorization", "Bearer "+tc.Token)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.Status || tc.Code != "" && !strings.Contains(w.Body.String(), `"code":"`+tc.Code+`"`) {
				t.Fatalf("got %d %s, want %d %s", w.Code, w.Body, tc.Status, tc.Code)
			}
		})
	}
}

func Controller(t *testing.T, h http.Handler, token, service string, wrongMethod, startLimit, missingStop int) {
	const ws = "/v1/workspaces/ws_test"
	HTTP(t, h,
		HTTPCase{"management auth", "GET", "/v1/capabilities", service, "", 401, "UNAUTHORIZED"},
		HTTPCase{"grant auth", "POST", "/v1/grants/verify", token, "{}", 401, "UNAUTHORIZED"},
		HTTPCase{"grant validation", "POST", "/v1/grants/verify", service, "{}", 403, "GRANT_REJECTED"},
		HTTPCase{"start method", "GET", ws + "/start", token, "", wrongMethod, ""},
		HTTPCase{"start limit", "POST", ws + "/start", token, "{}" + strings.Repeat(" ", startLimit), 400, "INVALID_REQUEST"},
		HTTPCase{"network limit", "POST", ws + "/network-policy", token, "{}" + strings.Repeat(" ", 32768), 400, "INVALID_REQUEST"},
		HTTPCase{"missing policy", "POST", ws + "/network-policy", token, "{}", 400, "INVALID_REQUEST"},
		HTTPCase{"agent path", "POST", ws + "/agent/v1/resume", token, "", 404, "NOT_FOUND_OR_FORBIDDEN"},
		HTTPCase{"agent method", "DELETE", ws + "/agent/v1/tasks", token, "", 404, "NOT_FOUND_OR_FORBIDDEN"},
		HTTPCase{"agent query", "GET", ws + "/agent/v1/tasks?limit=1&limit=2", token, "", 400, "INVALID_REQUEST"},
		HTTPCase{"inspect", "GET", ws, token, "", 200, ""},
		HTTPCase{"stop missing", "POST", ws + "/stop", token, "", missingStop, ""},
	)
}
