package gateway

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"
)

// MaintainSubscriptionLease renews only the gateway's read-only Vault lease.
// Provider logins and their refresh tokens remain under operator control.
func MaintainSubscriptionLease(ctx context.Context, c Config) error {
	if c.Subscriptions == nil {
		return nil
	}
	client, err := subscriptionVaultClient(c.SubscriptionVaultAddr, c.SubscriptionVaultCA)
	if err != nil {
		return err
	}
	renew := func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(c.SubscriptionVaultAddr, "/")+"/v1/auth/token/renew-self", strings.NewReader(`{}`))
		if err != nil {
			return ErrDenied
		}
		req.Header.Set("X-Vault-Token", c.SubscriptionVaultToken)
		res, err := client.Do(req)
		if err != nil {
			return ErrDenied
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return ErrDenied
		}
		return nil
	}
	maintainLease(ctx, 10*time.Minute, renew)
	return nil
}

func maintainLease(ctx context.Context, interval time.Duration, renew func(context.Context) error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if renew(ctx) != nil && ctx.Err() == nil {
			log.Print("subscription Vault lease renewal failed; operator recovery may be required")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
