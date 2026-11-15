package main

import (
	"agent-workbench/internal/dashboard"
	"agent-workbench/internal/shared"
	"errors"
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
		proxy := dashboard.OAuthProxyConfig{Origin: os.Getenv("DASHBOARD_PUBLIC_ORIGIN"), AuthURL: os.Getenv("OAUTH2_PROXY_AUTH_URL"), PublicURL: os.Getenv("OAUTH2_PROXY_PUBLIC_URL")}
		if proxy.Origin != "" || proxy.AuthURL != "" || proxy.PublicURL != "" {
			if cfg.ClientID != "" || cfg.APIKey != "" || cfg.RedirectURI != "" || len(cfg.AllowedEmails) != 0 {
				err = errors.New("choose WorkOS or OAuth2 Proxy authentication, not both")
			} else {
				h, err = dashboard.NewOAuthProxyHandler(h, proxy)
			}
		} else {
			h, err = dashboard.NewAuthKitHandler(h, cfg)
		}
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
