package workspacetools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"agent-workbench/internal/shared"
)

func Preview(ctx context.Context, args []string, out, errOut io.Writer) error {
	const usage = "usage: workbench-preview start|desktop|status|stop"
	if len(args) == 0 {
		return errors.New(usage)
	}
	action := args[0]
	flags := flag.NewFlagSet("workbench-preview "+action, flag.ContinueOnError)
	flags.SetOutput(errOut)
	directory := flags.String("directory", ".", "workspace application directory")
	port := flags.Int("port", 0, "application port")
	command := flags.String("command", "", "application command")
	key := flags.String("key", "", "stable retry key")
	id := flags.String("id", "", "service identifier")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	token := os.Getenv("WORKBENCH_WORKSPACE_GRANT")
	if token == "" {
		return errors.New("run this command inside an agent workspace")
	}
	client := &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	call := func(path string, body any, result any) error {
		var data io.Reader
		method := "GET"
		if body != nil {
			encoded, err := json.Marshal(body)
			if err != nil {
				return err
			}
			data = bytes.NewReader(encoded)
			method = "POST"
		}
		request, err := http.NewRequestWithContext(ctx, method, "http://127.0.0.1:8080/v1/"+path, data)
		if err != nil {
			return err
		}
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		if err != nil {
			return errors.New("workspace supervisor unavailable")
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return fmt.Errorf("preview request failed (HTTP %d)", response.StatusCode)
		}
		return json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(result)
	}
	printJSON := func(v any) error { return json.NewEncoder(out).Encode(v) }
	switch action {
	case "stop":
		if !strings.HasPrefix(*id, "service_") || !shared.ValidID(*id) {
			return errors.New("invalid service ID")
		}
		var result any
		if err := call("services/"+*id+"/revoke", map[string]any{}, &result); err != nil {
			return err
		}
		return printJSON(result)
	case "status":
		var result struct {
			Items []map[string]any `json:"items"`
		}
		if err := call("services", nil, &result); err != nil {
			return err
		}
		for _, item := range result.Items {
			delete(item, "logs")
		}
		return printJSON(result)
	case "start", "desktop":
		application := shared.ApplicationConfig{Directory: *directory, Command: *command, Port: *port}
		if action == "desktop" {
			application = shared.ApplicationConfig{Kind: "desktop", Directory: ".", Command: "workbench-desktop", Port: 6080}
		} else if *port < 1 || *port > 65535 || *command == "" {
			return errors.New("start requires --port and --command")
		}
		if *key == "" {
			*key = shared.NewID("agent-preview-")
		}
		var app map[string]any
		if err := call("services", map[string]any{"application": application, "ttl_seconds": 3600, "idempotency_key": *key}, &app); err != nil {
			return err
		}
		for attempt := 0; attempt < 30; attempt++ {
			var result struct {
				Items []map[string]any `json:"items"`
			}
			if err := call("services", nil, &result); err != nil {
				return err
			}
			for _, item := range result.Items {
				if item["id"] == app["id"] {
					app = item
					break
				}
			}
			if app["ready"] == true || app["state"] != "active" {
				break
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}
		}
		delete(app, "logs")
		if err := printJSON(app); err != nil {
			return err
		}
		if app["ready"] != true {
			return errors.New("application is not ready; inspect its logs in the workbench")
		}
		message := "Application is supervised inside this workspace. Open Web preview in the work conversation."
		if action == "desktop" {
			message = "Desktop is ready on DISPLAY=:99. Open Desktop in the work conversation."
		}
		_, err := fmt.Fprintln(out, message)
		return err
	default:
		return errors.New(usage)
	}
}
