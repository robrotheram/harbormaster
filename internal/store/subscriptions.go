package store

import (
	"context"
	"errors"
)

var ErrSubscriptionInUse = errors.New("SUBSCRIPTION_IN_USE")

func (s *Store) initSubscriptions(ctx context.Context) error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS user_subscriptions(owner TEXT PRIMARY KEY, credential TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS worker_subscriptions(workspace_id TEXT PRIMARY KEY, owner TEXT NOT NULL)`,
	} {
		if _, err := s.DB.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}
