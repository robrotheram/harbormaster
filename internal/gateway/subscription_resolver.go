package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func newSubscriptionResolver(origin, token string) (func(context.Context, string, string, string) (subscriptionCredential, error), error) {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || token == "" {
		return nil, ErrDenied
	}
	u.Path = "/internal/subscriptions/resolve"
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: NoRedirect}
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
