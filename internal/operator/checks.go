package operator

import (
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"time"
)

func CheckRuntimeHost(ctx context.Context, out, errOut io.Writer) error {
	cmd := exec.CommandContext(ctx, "docker", "compose", "config", "--format", "json")
	data, err := cmd.Output()
	if err != nil {
		return errors.New("could not inspect Compose configuration")
	}
	var config struct {
		Services map[string]struct {
			Environment map[string]string `json:"environment"`
		} `json:"services"`
	}
	if err = json.Unmarshal(data, &config); err != nil {
		return errors.New("invalid Compose configuration")
	}
	env := config.Services["runtime-controller"].Environment
	if env["RUNTIME_BACKEND"] != "firecracker" {
		return nil
	}
	socket := env["FIRECRACKER_SOCKET"]
	if socket == "" {
		socket = "/run/agent-workbench/controller.sock"
	}
	probe := exec.CommandContext(ctx, "docker", "compose", "run", "--rm", "--no-deps", "-T", "--entrypoint", "/usr/local/bin/healthcheck", "runtime-controller", "--unix-socket", socket)
	probe.Stdout = out
	probe.Stderr = errOut
	if probe.Run() != nil {
		return errors.New("Firecracker host controller unavailable; check its systemd service before starting Compose (no runtime fallback)")
	}
	return nil
}
func Smoke(ctx context.Context, args []string, out, errOut io.Writer) error {
	flags := flag.NewFlagSet("smoke", flag.ContinueOnError)
	flags.SetOutput(errOut)
	origin := os.Getenv("CONTROLPLANE_URL")
	if origin == "" {
		origin = "http://127.0.0.1:8090"
	}
	url := flags.String("url", origin, "controlplane origin")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected smoke arguments")
	}
	client, err := newClient(*url)
	if err != nil {
		return err
	}
	call := func(method, path string, body any) (map[string]any, error) {
		return client.call(ctx, method, path, body)
	}
	ws, err := call("POST", "/v1/workspaces", map[string]any{"project_id": "default", "idempotency_key": "functional-smoke-workspace"})
	if err != nil {
		return err
	}
	id, ok := ws["id"].(string)
	if !ok || !shared.ValidID(id) {
		return errors.New("invalid workspace response")
	}
	prefix := "/v1/workspaces/" + id
	key := shared.NewID("smoke_")
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 40*time.Second)
		defer cancel()
		_, _ = client.call(cleanup, "POST", prefix+"/stop", map[string]string{"idempotency_key": key + "_final_stop"})
	}()
	body := map[string]string{"instruction": "Synthetic functional smoke test.", "idempotency_key": key}
	task, err := call("POST", prefix+"/tasks", body)
	if err != nil {
		return err
	}
	replay, err := call("POST", prefix+"/tasks", body)
	if err != nil {
		return err
	}
	if task["id"] != replay["id"] {
		return errors.New("duplicate task on retry")
	}
	taskID, ok := task["id"].(string)
	if !ok || !shared.ValidID(taskID) {
		return errors.New("invalid task response")
	}
	path := prefix + "/tasks/" + taskID
	deadline := time.Now().Add(time.Minute)
	for (task["state"] == "queued" || task["state"] == "running" || task["state"] == "cancel_requested") && time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
		task, err = call("GET", path, nil)
		if err != nil {
			return err
		}
	}
	if task["state"] != "succeeded" {
		return errors.New("task did not succeed")
	}
	before, err := call("GET", path+"/result", nil)
	if err != nil {
		return err
	}
	content, _ := before["content"].(string)
	if !strings.Contains(content, "SYNTHETIC FIXTURE") {
		return errors.New("smoke requires fixture inference mode")
	}
	stopped, err := call("POST", prefix+"/stop", map[string]string{"idempotency_key": key + "_stop"})
	if err != nil {
		return err
	}
	if stopped["state"] != "STOPPED" {
		return errors.New("workspace did not stop")
	}
	after, err := call("GET", path+"/result", nil)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(before, after) {
		return errors.New("result lost across stop/start")
	}
	current, err := call("GET", prefix, nil)
	if err != nil {
		return err
	}
	if current["isolation_status"] != "unverified" || current["network_status"] != "unverified" {
		return errors.New("unexpected isolation claim")
	}
	_, err = fmt.Fprintln(out, "Functional smoke passed: task deduplication, fixture inference and retained results. This does not verify VM or network isolation.")
	return err
}
