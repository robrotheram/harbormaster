package main

import (
	"agent-workbench/internal/api"
	"agent-workbench/internal/gateway"
	"agent-workbench/internal/runtime/controllerclient"
	"agent-workbench/internal/shared"
	"agent-workbench/internal/store"
	"context"
	"encoding/json"
	"errors"
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
		if errors.Is(e, store.ErrPreReleaseSchema) {
			slog.Error(store.ErrPreReleaseSchema.Error())
		} else {
			slog.Error("metadata database unavailable")
		}
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
	subscriptionToken, err := shared.Secret("SUBSCRIPTION_GATEWAY_TOKEN")
	if err != nil {
		slog.Error("subscription gateway credential unavailable")
		os.Exit(1)
	}
	controller := shared.Env("CONTROLLER_URL", "http://runtime-controller:8080")
	var verifySubscriptionGrant func(context.Context, string, string, string) bool
	if subscriptionToken != "" {
		serviceToken, secretErr := shared.Secret("GATEWAY_SERVICE_TOKEN")
		verify, verifierErr := gateway.NewVerifier(controller, serviceToken)
		if secretErr != nil || verifierErr != nil || len(subscriptionToken) < 32 || subscriptionToken == serviceToken || subscriptionToken == token || subscriptionToken == runtimeToken {
			slog.Error("subscription resolver requires its own credential and runtime verification")
			os.Exit(1)
		}
		verifySubscriptionGrant = func(ctx context.Context, grant, workspace, project string) bool {
			id, err := verify(ctx, grant, "inference")
			return err == nil && id.WorkspaceID == workspace && id.ProjectID == project
		}
	}
	var credentialKeys api.CredentialKeys
	key, keyErr := shared.Secret("CREDENTIAL_ENCRYPTION_KEY")
	keyring, ringErr := shared.Secret("CREDENTIAL_ENCRYPTION_KEYRING")
	legacy, legacyErr := shared.Secret("CREDENTIAL_LEGACY_DECRYPTION_KEY")
	if keyErr != nil || ringErr != nil || legacyErr != nil || (key != "" && keyring != "") {
		slog.Error("credential encryption configuration unavailable")
		os.Exit(1)
	}
	if key != "" {
		credentialKeys = api.CredentialKeys{Active: "default", Keys: map[string]string{"default": key}}
	}
	if keyring != "" && gateway.Decode([]byte(keyring), &credentialKeys) != nil {
		slog.Error("invalid credential encryption keyring")
		os.Exit(1)
	}
	if (key != "" || keyring != "") && credentialKeys.Validate(token) != nil {
		slog.Error("invalid credential encryption keyring")
		os.Exit(1)
	}
	if legacy != "" {
		if credentialKeys.Legacy != "" {
			slog.Error("configure the legacy decryption key in one place")
			os.Exit(1)
		}
		credentialKeys.Legacy = legacy
	}
	creators := make(map[string]bool)
	for _, email := range strings.FieldsFunc(os.Getenv("PROJECT_CREATOR_EMAILS"), func(r rune) bool { return r == ',' || r == '\n' }) {
		creators[strings.ToLower(strings.TrimSpace(email))] = true
	}
	keys := map[string]string{}
	used := map[string]bool{token: true, runtimeToken: true}
	for _, name := range []string{"DASHBOARD_READ_TOKEN", "DASHBOARD_HUMAN_SIGNING_KEY", "PROJECT_SIGNING_KEY", "DASHBOARD_LOCAL_OPERATOR_TOKEN"} {
		value, err := shared.Secret(name)
		if err != nil || (value != "" && (len(value) < 32 || used[value] || strings.ContainsAny(value, " \r\n\t"))) {
			slog.Error("invalid scoped credential configuration", "name", name)
			os.Exit(1)
		}
		keys[name] = value
		if value != "" {
			used[value] = true
		}
	}
	if legacy := credentialKeys.Legacy; legacy != "" && (len(legacy) < 32 || used[legacy] || strings.ContainsAny(legacy, " \r\n\t")) {
		slog.Error("invalid legacy decryption key")
		os.Exit(1)
	}
	for _, key := range credentialKeys.Keys {
		if used[key] || key == credentialKeys.Legacy {
			slog.Error("credential encryption keys must be distinct from other service credentials")
			os.Exit(1)
		}
	}
	server := api.New(db, controllerclient.NewClient(controller, runtimeToken), api.Config{Version: os.Getenv("HARBORMASTER_VERSION"), DashboardReadToken: keys["DASHBOARD_READ_TOKEN"], DashboardLocalOperatorToken: keys["DASHBOARD_LOCAL_OPERATOR_TOKEN"], HumanSigningKey: keys["DASHBOARD_HUMAN_SIGNING_KEY"], ProjectSigningKey: keys["PROJECT_SIGNING_KEY"], ProjectCreatorEmails: creators, MaxProjectsPerUser: int(shared.IntEnv("MAX_PROJECTS_PER_USER", 5)), MaxRunningPerUser: int(shared.IntEnv("MAX_RUNNING_WORKSPACES_PER_USER", 2)), CredentialKeys: credentialKeys, RequireScopedCredentials: true, SubscriptionGatewayToken: subscriptionToken, RequireAgentSelection: os.Getenv("GATEWAY_MODE") == "subscription", VerifySubscriptionGrant: verifySubscriptionGrant, ProvisionProjectNamespaces: shared.Env("PROVISION_PROJECT_NAMESPACES", "false") == "true", SharedWorkspaceID: os.Getenv("SHARED_WORKSPACE_ID"), WorkspaceMemory: shared.IntEnv("WORKSPACE_MEMORY_BYTES", 1<<30), MemoryQuota: shared.IntEnv("MEMORY_QUOTA_BYTES", 4<<30), Token: token, ProjectTokens: projectTokens, HealthEndpoints: map[string]string{"Runtime controller": shared.Env("CONTROLLER_URL", "http://runtime-controller:8080") + "/healthz", "Inference gateway": "http://inference-gateway:8080/healthz", "Credential broker": "http://credential-broker:8080/healthz", "GitHub egress gateway": "http://egress-gateway:8080/healthz", "MCP": "http://mcp:8080/healthz"}})
	if os.Getenv("SHARED_WORKSPACE_ID") != "" {
		if _, err := server.EnsureSharedWorkspace(ctx); err != nil {
			slog.Error("shared workspace provisioning failed")
			os.Exit(1)
		}
	}
	var workers sync.WaitGroup
	workers.Go(func() { server.Run(ctx) })
	workers.Go(func() {
		if shared.ServeContext(ctx, shared.Env("READ_LISTEN_ADDR", ":8081"), server.DashboardHandler()) != nil {
			slog.Error("dashboard read listener stopped")
			cancel()
		}
	})
	workers.Go(func() {
		if shared.ServeContext(ctx, shared.Env("LISTEN_ADDR", ":8080"), server.Handler()) != nil {
			slog.Error("control API stopped")
			cancel()
		}
	})
	<-ctx.Done()
	workers.Wait()
}
