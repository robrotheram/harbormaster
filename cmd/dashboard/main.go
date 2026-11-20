package main

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"agent-workbench/internal/dashboard"
	"agent-workbench/internal/shared"
)

func main() {
	h, err := handler()
	if err != nil {
		slog.Error("invalid dashboard configuration", "error", err)
		os.Exit(1)
	}
	defaultAddress := ":8080"
	if os.Getenv("DASHBOARD_AUTH") == "local" {
		defaultAddress = "127.0.0.1:8080"
	}
	if err := shared.Serve(shared.Env("DASHBOARD_LISTEN_ADDR", defaultAddress), h); err != nil {
		slog.Error("dashboard stopped")
		os.Exit(1)
	}
}

func handler() (http.Handler, error) {
	cfg, err := dashboard.AuthConfigFromEnv()
	if err != nil {
		return nil, errors.New("WorkOS configuration unavailable")
	}
	proxy := dashboard.OAuthProxyConfig{
		Origin:    os.Getenv("DASHBOARD_PUBLIC_ORIGIN"),
		AuthURL:   os.Getenv("OAUTH2_PROXY_AUTH_URL"),
		PublicURL: os.Getenv("OAUTH2_PROXY_PUBLIC_URL"),
		AllowedEmails: strings.FieldsFunc(os.Getenv("OAUTH2_PROXY_ALLOWED_EMAILS"), func(r rune) bool {
			return r == ',' || r == '\n'
		}),
	}
	authKit := cfg.ClientID != "" || cfg.APIKey != "" || cfg.RedirectURI != "" || len(cfg.AllowedEmails) > 0
	oauthProxy := proxy.AuthURL != "" || proxy.PublicURL != "" || len(proxy.AllowedEmails) > 0
	if authKit && oauthProxy {
		return nil, errors.New("choose WorkOS or OAuth2 Proxy authentication")
	}
	readToken, readErr := shared.Secret("DASHBOARD_READ_TOKEN")
	token, tokenErr := shared.Secret("CONTROL_TOKEN")
	signingKey, signingErr := shared.Secret("DASHBOARD_HUMAN_SIGNING_KEY")
	if readErr != nil || tokenErr != nil || signingErr != nil {
		return nil, errors.New("dashboard credential unavailable")
	}
	readURL := shared.Env("CONTROLPLANE_READ_URL", "http://controlplane:8081")
	controlURL := shared.Env("CONTROLPLANE_URL", "http://controlplane:8080")
	switch {
	case authKit || oauthProxy || proxy.Origin != "":
		if token != "" {
			return nil, errors.New("hosted dashboard must not receive the control token")
		}
		var h http.Handler
		if signingKey != "" {
			h, err = dashboard.NewHostedInteractiveHandler(readURL, controlURL, readToken, signingKey)
		} else {
			h, err = dashboard.NewHandler(readURL, readToken)
		}
		if err != nil {
			return nil, err
		}
		if authKit {
			return dashboard.NewAuthKitHandler(h, cfg)
		}
		return dashboard.NewOAuthProxyHandler(h, proxy)
	case signingKey != "":
		return nil, errors.New("dashboard signing key requires hosted authentication")
	case token != "":
		if os.Getenv("DASHBOARD_AUTH") != "local" {
			return nil, errors.New("interactive dashboard requires explicit DASHBOARD_AUTH=local")
		}
		password, err := shared.Secret("DASHBOARD_LOCAL_PASSWORD")
		if err != nil {
			return nil, errors.New("local dashboard password unavailable")
		}
		h, err := dashboard.NewInteractiveHandler(readURL, controlURL, token)
		if err != nil {
			return nil, err
		}
		h, err = dashboard.NewAuthKitHandler(h, cfg)
		if err != nil {
			return nil, err
		}
		return dashboard.NewLocalAuthHandler(h, password)
	default:
		return dashboard.NewHandler(readURL, readToken)
	}
}
