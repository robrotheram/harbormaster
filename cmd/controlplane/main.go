package main

import (
	"agent-workbench/internal/api"
	"agent-workbench/internal/runtime/controllerclient"
	"agent-workbench/internal/shared"
	"agent-workbench/internal/store"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
)

func main() {
	token, e := shared.Secret("CONTROL_TOKEN")
	if e != nil || token == "" {
		slog.Error("control token required")
		os.Exit(1)
	}
	runtimeToken, e := shared.Secret("CONTROLLER_TOKEN")
	if e != nil || runtimeToken == "" {
		slog.Error("controller token required")
		os.Exit(1)
	}
	dsn, e := shared.Secret("DATABASE_URL")
	if e != nil || dsn == "" {
		slog.Error("database connection required")
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	db, e := store.Open(ctx, dsn)
	if e != nil {
		slog.Error("metadata database unavailable")
		os.Exit(1)
	}
	defer db.DB.Close()
	projectTokens := map[string]string{}
	if path := os.Getenv("PROJECT_TOKENS_FILE"); path != "" {
		f, err := os.Open(path)
		if err != nil {
			slog.Error("project token configuration unavailable")
			os.Exit(1)
		}
		decoder := json.NewDecoder(io.LimitReader(f, 1<<20))
		err = decoder.Decode(&projectTokens)
		f.Close()
		if err != nil {
			slog.Error("invalid project token configuration")
			os.Exit(1)
		}
		for credential, project := range projectTokens {
			if len(credential) < 32 || strings.ContainsAny(credential, " \r\n\t") || credential == token || !shared.ValidID(project) {
				slog.Error("invalid project token configuration")
				os.Exit(1)
			}
		}
	}
	server := api.New(db, controllerclient.NewClient(shared.Env("CONTROLLER_URL", "http://runtime-controller:8080"), runtimeToken), api.Config{ProvisionProjectNamespaces: shared.Env("PROVISION_PROJECT_NAMESPACES", "false") == "true", SharedWorkspaceID: os.Getenv("SHARED_WORKSPACE_ID"), WorkspaceMemory: shared.IntEnv("WORKSPACE_MEMORY_BYTES", 1<<30), MemoryQuota: shared.IntEnv("MEMORY_QUOTA_BYTES", 4<<30), Token: token, ProjectTokens: projectTokens, HealthEndpoints: map[string]string{"Runtime controller": shared.Env("CONTROLLER_URL", "http://runtime-controller:8080") + "/healthz", "Inference gateway": "http://inference-gateway:8080/healthz", "Credential broker": "http://credential-broker:8080/healthz", "GitHub egress gateway": "http://egress-gateway:8080/healthz", "MCP": "http://mcp:8080/healthz"}})
	if os.Getenv("SHARED_WORKSPACE_ID") != "" {
		if _, err := server.EnsureSharedWorkspace(ctx); err != nil {
			slog.Error("shared workspace provisioning failed")
			os.Exit(1)
		}
	}
	var workers sync.WaitGroup
	workers.Add(3)
	go func() { defer workers.Done(); server.Run(ctx) }()
	go func() {
		defer workers.Done()
		if shared.ServeContext(ctx, shared.Env("READ_LISTEN_ADDR", ":8081"), server.DashboardHandler()) != nil {
			slog.Error("dashboard read listener stopped")
			cancel()
		}
	}()
	go func() {
		defer workers.Done()
		if shared.ServeContext(ctx, shared.Env("LISTEN_ADDR", ":8080"), server.Handler()) != nil {
			slog.Error("control API stopped")
			cancel()
		}
	}()
	<-ctx.Done()
	workers.Wait()
}
