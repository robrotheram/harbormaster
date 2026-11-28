package gateway

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func newSubscriptionResolver(origin, token, caFile string) (func(context.Context, string, string, string) (subscriptionCredential, error), error) {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || u.Scheme != "https" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || token == "" {
		return nil, ErrDenied
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if caFile != "" {
		pem, err := os.ReadFile(caFile)
		if err != nil {
			return nil, ErrDenied
		}
		tlsConfig.RootCAs = x509.NewCertPool()
		if !tlsConfig.RootCAs.AppendCertsFromPEM(pem) {
			return nil, ErrDenied
		}
	}
	u.Path = "/internal/subscriptions/resolve"
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: tlsConfig}, CheckRedirect: NoRedirect}
	return func(ctx context.Context, workspace, provider, grant string) (subscriptionCredential, error) {
		body, _ := json.Marshal(map[string]string{"workspace_id": workspace, "provider": provider, "grant": grant})
		req, err := http.NewRequestWithContext(ctx, "POST", u.String(), strings.NewReader(string(body)))
		if err != nil {
			return subscriptionCredential{}, ErrDenied
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		res, err := client.Do(req)
		if err != nil {
			return subscriptionCredential{}, ErrDenied
		}
		defer res.Body.Close()
		b, err := ReadBounded(res.Body, 64<<10)
		var c subscriptionCredential
		if err != nil || res.StatusCode != 200 || json.Unmarshal(b, &c) != nil {
			return subscriptionCredential{}, ErrDenied
		}
		return c, nil
	}, nil
}
