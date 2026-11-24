package dashboard

import (
	"time"

	"agent-workbench/internal/shared"
)

func limitConnections[T any](items map[string]T, human *shared.HumanIdentity, limit int, binding func(T) (*shared.HumanIdentity, time.Time), discard func(T)) {
	now := time.Now()
	count := 0
	var oldestKey string
	var oldest time.Time
	for key, item := range items {
		owner, expiry := binding(item)
		if !now.Before(expiry) {
			if discard != nil {
				discard(item)
			}
			delete(items, key)
			continue
		}
		if (owner == nil && human == nil) || (owner != nil && human != nil && owner.ID == human.ID) {
			count++
			if oldestKey == "" || expiry.Before(oldest) {
				oldestKey, oldest = key, expiry
			}
		}
	}
	if limit > 0 && count >= limit {
		if discard != nil {
			discard(items[oldestKey])
		}
		delete(items, oldestKey)
	}
}
