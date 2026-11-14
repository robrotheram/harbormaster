package main

import (
	"agent-workbench/internal/dashboard"
	"agent-workbench/internal/shared"
	"log/slog"
	"os"
)

func main() {
	h, err := dashboard.NewHandler(shared.Env("CONTROLPLANE_READ_URL", "http://controlplane:8081"))
	if token, e := shared.Secret("CONTROL_TOKEN"); e == nil && token != "" {
		h, err = dashboard.NewInteractiveHandler(shared.Env("CONTROLPLANE_READ_URL", "http://controlplane:8081"), shared.Env("CONTROLPLANE_URL", "http://controlplane:8080"), token)
	}
	if err != nil {
		slog.Error("invalid dashboard configuration")
		os.Exit(1)
	}
	cfg, err := dashboard.AuthConfigFromEnv()
	if err == nil {
		h, err = dashboard.NewAuthKitHandler(h, cfg)
	}
	if err != nil {
		slog.Error("invalid dashboard authentication configuration", "error", err)
		os.Exit(1)
	}
	if err := shared.Serve(":8080", h); err != nil {
		slog.Error("dashboard stopped")
		os.Exit(1)
	}
}
