package store

import (
	"agent-workbench/internal/shared"
	"context"
	"net/http"
)

var ErrSubscriptionInUse = &shared.CodedError{Status: http.StatusConflict, Code: "SUBSCRIPTION_IN_USE"}

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
