package egress

import (
	"context"
	"crypto/sha256"
	"sync"
	"time"

	"agent-workbench/internal/gateway"
)

type cachedGrant struct {
	identity gateway.Identity
	expires  time.Time
}

type admission struct {
	mu     sync.Mutex
	grants map[[32]byte]cachedGrant
	active map[string]int
}

func (a *admission) verifier(verify gateway.Verifier) gateway.Verifier {
	if verify == nil {
		return nil
	}
	return func(ctx context.Context, token, audience string) (gateway.Identity, error) {
		// Audience is fixed by the egress handler, but include it in the key.
		key := sha256.Sum256([]byte(audience + "\x00" + token))
		a.mu.Lock()
		item, ok := a.grants[key]
		a.mu.Unlock()
		now := time.Now()
		if ok && now.Before(item.expires) {
			return item.identity, nil
		}
		identity, err := verify(ctx, token, audience)
		if err == nil && gateway.ValidIdentity(identity) {
			a.mu.Lock()
			if len(a.grants) >= 2048 {
				for key, value := range a.grants {
					if !now.Before(value.expires) {
						delete(a.grants, key)
					}
				}
			}
			if len(a.grants) < 2048 {
				a.grants[key] = cachedGrant{identity, now.Add(time.Second)}
			}
			a.mu.Unlock()
		}
		return identity, err
	}
}

func (a *admission) acquire(workspace string) (release func(), ok bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.active[workspace] >= 24 {
		return nil, false
	}
	a.active[workspace]++
	return func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.active[workspace] == 1 {
			delete(a.active, workspace)
		} else {
			a.active[workspace]--
		}
	}, true
}
