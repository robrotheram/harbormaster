package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agent-workbench/internal/shared"
)

func TestProjectSeedScopeAndUnpauseOnFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "copy", true: "failed-copy"}[fail], func(t *testing.T) {
			paused, resumed := false, false
			received := map[string]string{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path := strings.TrimPrefix(r.URL.Path, "/v1.47")
				switch {
				case path == "/containers/testawb-ws_parent/json":
					shared.JSON(w, 200, map[string]any{"Id": "source", "State": map[string]any{"Running": true}, "Config": map[string]any{"Labels": map[string]string{"workbench.deployment": "testawb", "workbench.workspace": "ws_parent"}}, "HostConfig": map[string]string{"Runtime": "runc"}, "NetworkSettings": map[string]any{"Networks": map[string]any{"testawb-ws_parent": map[string]string{"IPAddress": "172.30.0.5"}}}})
				case strings.Contains(path, "/stats"):
					shared.JSON(w, 200, map[string]any{})
				case path == "/containers/source/pause":
					paused = true
					w.WriteHeader(204)
				case path == "/containers/source/unpause":
					paused = false
					resumed = true
					w.WriteHeader(204)
				case path == "/containers/target/archive" && r.Method == "GET":
					w.WriteHeader(404)
				case path == "/containers/source/archive" && r.Method == "GET":
					if !paused || r.URL.Query().Get("path") != "/data/files" {
						t.Error("source copy escaped its scope")
					}
					if fail {
						w.WriteHeader(500)
						return
					}
					writer := tar.NewWriter(w)
					_ = writer.WriteHeader(&tar.Header{Name: "files/example.txt", Mode: 0600, Size: 7})
					_, _ = writer.Write([]byte("project"))
					_ = writer.Close()
				case path == "/containers/target/archive" && r.Method == "PUT":
					if r.URL.Query().Get("path") != "/data" {
						t.Error("wrong target scope")
					}
					reader := tar.NewReader(r.Body)
					for {
						header, err := reader.Next()
						if err == io.EOF {
							break
						}
						if err != nil {
							t.Error(err)
							break
						}
						body, _ := io.ReadAll(reader)
						received[header.Name] = string(body)
					}
					w.WriteHeader(200)
				default:
					t.Error("unexpected Docker operation: " + r.Method + " " + path)
					w.WriteHeader(500)
				}
			}))
			defer server.Close()
			c, err := NewController(&Docker{HTTP: server.Client(), Base: server.URL + "/v1.47"}, ControllerConfig{Token: strings.Repeat("a", 32), GatewayServiceToken: strings.Repeat("b", 32), GrantSecret: strings.Repeat("c", 32), Deployment: "testawb", Image: "fixed", RuntimeType: "docker"})
			if err != nil {
				t.Fatal(err)
			}
			c.agentHTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				body := `{"status":"ok"}`
				if r.URL.Path == "/v1/activity" {
					var data bytes.Buffer
					data.WriteString(`{"active_runs":0,"background_jobs":0,"known":true,"last_activity":"` + time.Now().UTC().Format(time.RFC3339Nano) + `"}`)
					body = data.String()
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
			})}
			target := containerInfo{ID: "target"}
			target.Config.Labels = map[string]string{"workbench.parent": "ws_parent"}
			err = c.seedFromParent(context.Background(), target, "ws_parent")
			if fail != (err != nil) {
				t.Fatalf("seed error mismatch: %v", err)
			}
			if paused || !resumed {
				t.Fatal("source left frozen")
			}
			if !fail && (received["files/example.txt"] != "project" || received[".workbench-seeded"] != "ws_parent" || len(received) != 2) {
				t.Fatal("project-only copy failed")
			}
		})
	}
}
