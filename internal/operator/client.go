package operator

import (
	"agent-workbench/internal/shared"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type apiClient struct {
	base, token string
	client      *http.Client
}

func newClient(origin string) (*apiClient, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("invalid controlplane URL")
	}
	if u.Scheme != "https" && (u.Scheme != "http" || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1")) {
		return nil, errors.New("use HTTPS except for loopback development")
	}
	path := os.Getenv("CONTROL_TOKEN_FILE")
	if path == "" {
		path = ".local/secrets/control_token"
	}
	token, err := os.ReadFile(path)
	if err != nil || len(bytes.TrimSpace(token)) == 0 {
		return nil, errors.New("control token unavailable; provision credentials first")
	}
	return &apiClient{strings.TrimRight(origin, "/"), string(bytes.TrimSpace(token)), &http.Client{Timeout: 40 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *apiClient) call(ctx context.Context, method, path string, payload any) (map[string]any, error) {
	var data io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		data = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, data)
	if err != nil {
		return nil, errors.New("invalid controlplane request")
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(req)
	if err != nil {
		return nil, errors.New("controlplane unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("controlplane request failed (HTTP %d)", response.StatusCode)
	}
	var result map[string]any
	err = json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&result)
	return result, err
}

// Accept options before or after positional arguments to retain the operator CLI's usage.
func parseFlags(flags *flag.FlagSet, args []string) ([]string, error) {
	options, positionals := []string{}, []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "-") {
			options = append(options, arg)
			if !strings.Contains(arg, "=") {
				if i+1 >= len(args) {
					return nil, fmt.Errorf("missing value for %s", arg)
				}
				i++
				options = append(options, args[i])
			}
		} else {
			positionals = append(positionals, arg)
		}
	}
	if err := flags.Parse(options); err != nil {
		return nil, err
	}
	return positionals, nil
}
func APICommand(ctx context.Context, group string, args []string, out, errOut io.Writer) error {
	flags := flag.NewFlagSet(group, flag.ContinueOnError)
	flags.SetOutput(errOut)
	origin := os.Getenv("CONTROLPLANE_URL")
	if origin == "" {
		origin = "http://127.0.0.1:8090"
	}
	endpoint := flags.String("url", origin, "controlplane origin")
	key := flags.String("key", "", "stable retry key")
	provider := flags.String("provider", "", "codex or claude")
	values, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(values) == 0 {
		return errors.New("an operation is required")
	}
	action, values := values[0], values[1:]
	if *key == "" {
		*key = shared.NewID("request_")
	}
	method, path, payload := "GET", "/v1/workspaces", any(nil)
	counts := map[string]int{"create": 0, "list": 0, "usage": 0, "get": 1, "start": 1, "stop": 1, "submit": 2, "status": 2, "result": 2, "cancel": 2}
	if group == "project" {
		counts = map[string]int{"create": 1, "list": 0, "access": 1}
		path = "/v1/projects"
	}
	count, ok := counts[action]
	if !ok || len(values) != count {
		return errors.New("invalid operation or argument count")
	}
	if group == "workspace" {
		ids := len(values)
		if action == "submit" {
			ids = 1
		}
		for _, id := range values[:ids] {
			if !shared.ValidID(id) {
				return errors.New("invalid identifier")
			}
		}
		switch action {
		case "create":
			method, payload = "POST", map[string]any{"project_id": "default", "idempotency_key": *key}
		case "usage":
			path = "/v1/usage"
		case "list":
		default:
			path += "/" + values[0]
			switch action {
			case "start", "stop":
				path += "/" + action
				method, payload = "POST", map[string]any{"idempotency_key": *key}
			case "submit":
				if *provider != "" && *provider != "codex" && *provider != "claude" {
					return errors.New("invalid provider")
				}
				path += "/tasks"
				method, payload = "POST", map[string]any{"instruction": values[1], "idempotency_key": *key, "provider": *provider}
			case "status", "result", "cancel":
				path += "/tasks/" + values[1]
				if action != "status" {
					path += "/" + action
				}
				if action == "cancel" {
					method, payload = "POST", map[string]any{"idempotency_key": *key}
				}
			}
		}
	} else {
		switch action {
		case "create":
			method, payload = "POST", map[string]any{"name": values[0], "idempotency_key": *key}
		case "access":
			if !shared.ValidID(values[0]) {
				return errors.New("invalid project ID")
			}
			path += "/" + values[0] + "/agent-access"
			method, payload = "POST", map[string]any{"ttl_seconds": 86400}
		}
	}
	client, err := newClient(*endpoint)
	if err != nil {
		return err
	}
	result, err := client.call(ctx, method, path, payload)
	if err != nil {
		return err
	}
	if group == "project" && action == "access" {
		token, ok := result["access_token"].(string)
		if !ok || token == "" {
			return errors.New("project access response was invalid")
		}
		dir := ".local/project-access"
		if err = os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		if err = os.Chmod(dir, 0700); err != nil {
			return err
		}
		target := filepath.Join(dir, values[0]+".json")
		data, err := json.MarshalIndent(map[string]any{"project_id": values[0], "expires_at": result["expires_at"], "mcpServers": map[string]any{"workbench": map[string]any{"type": "http", "url": "http://127.0.0.1:8082/mcp", "headers": map[string]string{"Authorization": "Bearer " + token}}}}, "", "  ")
		if err != nil {
			return err
		}
		if err = privateWrite(target, data); err != nil {
			return err
		}
		if err = os.Chmod(target, 0600); err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, "Project-scoped MCP configuration saved privately:", target)
		return err
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}
