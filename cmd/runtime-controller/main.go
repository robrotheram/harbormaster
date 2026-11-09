package main

import (
	"agent-workbench/internal/runtime/docker"
	"agent-workbench/internal/runtime/firecracker"
	"agent-workbench/internal/runtime/kubernetes"
	"agent-workbench/internal/shared"
	"log/slog"
	"os"
)

func main() {
	backend := shared.Env("RUNTIME_BACKEND", "docker")
	if backend == "firecracker" {
		if err := firecracker.ServeFirecrackerBridge(); err != nil {
			slog.Error("Firecracker host controller unavailable", "error", err)
			os.Exit(1)
		}
		return
	}
	if backend != "docker" && backend != "kubernetes" {
		slog.Error("unsupported runtime backend; refusing Docker fallback")
		os.Exit(1)
	}
	if shared.Env("RUNTIME_BACKEND", "docker") == "kubernetes" {
		config, err := kubernetes.KubernetesConfigFromEnv()
		if err != nil {
			slog.Error("invalid Kubernetes runtime configuration")
			os.Exit(1)
		}
		provider, err := kubernetes.NewKubernetes(config)
		if err != nil {
			slog.Error("invalid Kubernetes runtime configuration")
			os.Exit(1)
		}
		if shared.Serve(shared.Env("LISTEN_ADDR", ":8080"), provider.Handler()) != nil {
			slog.Error("runtime controller stopped")
			os.Exit(1)
		}
		return
	}
	token, e := shared.Secret("CONTROLLER_TOKEN")
	if e != nil {
		slog.Error("controller token unavailable")
		os.Exit(1)
	}
	service, e := shared.Secret("GATEWAY_SERVICE_TOKEN")
	if e != nil {
		slog.Error("gateway service token unavailable")
		os.Exit(1)
	}
	secret, e := shared.Secret("GRANT_SECRET")
	if e != nil {
		slog.Error("grant signing secret unavailable")
		os.Exit(1)
	}
	c, e := docker.NewController(docker.NewDocker(shared.Env("DOCKER_SOCKET", "/var/run/docker.sock")), docker.ControllerConfig{EgressContainer: shared.Env("EGRESS_CONTAINER", ""), MemoryLimit: shared.IntEnv("WORKSPACE_MEMORY_BYTES", 1<<30), Console: shared.Env("WORKSPACE_T3", "true") == "true", DefaultProvider: shared.Env("WORKSPACE_AGENT", ""), CodexModel: shared.Env("CODEX_MODEL", ""), ClaudeModel: shared.Env("CLAUDE_MODEL", ""), Token: token, GatewayServiceToken: service, GrantSecret: secret, Deployment: shared.Env("DEPLOYMENT_ID", "awb"), Image: shared.Env("WORKSPACE_IMAGE", "agent-workbench-workspace:dev"), RuntimeType: shared.Env("WORKSPACE_RUNTIME", "docker"), RequireVerified: shared.Env("REQUIRE_VERIFIED_ISOLATION", "false") == "true", SelfContainer: shared.Env("CONTROLLER_CONTAINER", "awb-runtime-controller"), InferenceContainer: shared.Env("INFERENCE_CONTAINER", "awb-inference-gateway"), BrokerContainer: shared.Env("BROKER_CONTAINER", "awb-credential-broker"), Model: shared.Env("INFERENCE_MODEL", "fixture")})
	if e != nil {
		slog.Error("invalid runtime controller configuration")
		os.Exit(1)
	}
	if shared.Serve(shared.Env("LISTEN_ADDR", ":8080"), c.Handler()) != nil {
		slog.Error("runtime controller stopped")
		os.Exit(1)
	}
}
