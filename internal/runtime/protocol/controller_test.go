package protocol

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agent-workbench/internal/shared"
)

func TestControllerProxyPreservesStreaming(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		name := "explicit routes"
		if fallback {
			name = "workspace action fallback"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			release := make(chan struct{})
			guest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil || string(body) != "request" || r.Method != "PATCH" ||
					r.URL.RequestURI() != "/v1/t3-desktop/session?size=large" ||
					r.Header.Get("Authorization") != "Bearer guest-token" {
					t.Errorf("unexpected guest request: %s %s, body %q, error %v", r.Method, r.URL, body, err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				w.WriteHeader(http.StatusAccepted)
				_, _ = io.WriteString(w, "first")
				w.(http.Flusher).Flush()
				select {
				case <-release:
					_, _ = io.WriteString(w, "second")
				case <-r.Context().Done():
				}
			}))
			defer guest.Close()
			noOp := func(http.ResponseWriter, *http.Request) {}
			controller := httptest.NewServer(NewControllerHandler(Backend{
				Token: "controller-token", Health: noOp, VerifyGrant: noOp,
				WorkspaceActionFallback: fallback,
				Proxy: func(w http.ResponseWriter, r *http.Request, id, path, prefix string) {
					if id != "ws_test" {
						t.Errorf("unexpected workspace %q", id)
					}
					shared.Proxy(w, r, guest.URL, prefix+path, "guest-token")
				},
			}))
			defer controller.Close()
			defer cancel()
			r, err := http.NewRequestWithContext(ctx, "PATCH", controller.URL+"/v1/workspaces/ws_test/desktop/session?size=large", strings.NewReader("request"))
			if err != nil {
				t.Fatal(err)
			}
			r.Header.Set("Authorization", "Bearer controller-token")
			resp, err := controller.Client().Do(r)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			first := make([]byte, len("first"))
			if _, err := io.ReadFull(resp.Body, first); err != nil || string(first) != "first" || resp.StatusCode != http.StatusAccepted {
				t.Fatalf("first chunk: status %d, body %q, error %v", resp.StatusCode, first, err)
			}
			close(release)
			if rest, err := io.ReadAll(resp.Body); err != nil || string(rest) != "second" {
				t.Fatalf("second chunk: body %q, error %v", rest, err)
			}
		})
	}
}
