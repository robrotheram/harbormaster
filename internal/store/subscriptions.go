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
func (s *Store) Subscription(ctx context.Context, owner string) (string, error) {
	var v string
	err := s.DB.QueryRowContext(ctx, s.query(`SELECT credential FROM user_subscriptions WHERE owner=?`), owner).Scan(&v)
	return v, err
}
func (s *Store) SaveSubscription(ctx context.Context, owner, value string) error {
	_, err := s.DB.ExecContext(ctx, s.query(`INSERT INTO user_subscriptions(owner,credential) VALUES(?,?) ON CONFLICT(owner) DO UPDATE SET credential=excluded.credential`), owner, value)
	return err
}
func (s *Store) SubscriptionOwner(ctx context.Context, workspace string) (string, error) {
	var owner string
	err := s.DB.QueryRowContext(ctx, s.query(`SELECT owner FROM worker_subscriptions WHERE workspace_id=?`), workspace).Scan(&owner)
	return owner, err
}
func (s *Store) LoadSubscription(ctx context.Context, owner, workspace string) error {
	res, err := s.DB.ExecContext(ctx, s.query(`INSERT INTO worker_subscriptions(workspace_id,owner) VALUES(?,?) ON CONFLICT(workspace_id) DO UPDATE SET owner=excluded.owner WHERE worker_subscriptions.owner=excluded.owner`), workspace, owner)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrSubscriptionInUse
	}
	return nil
}
func (s *Store) UnloadSubscription(ctx context.Context, owner, workspace string) error {
	_, err := s.DB.ExecContext(ctx, s.query(`DELETE FROM worker_subscriptions WHERE workspace_id=? AND owner=?`), workspace, owner)
	return err
}
func (s *Store) DeleteSubscription(ctx context.Context, owner string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{`DELETE FROM worker_subscriptions WHERE owner=?`, `DELETE FROM user_subscriptions WHERE owner=?`} {
		if _, err = tx.ExecContext(ctx, s.query(q), owner); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *Store) SubscriptionWorkers(ctx context.Context, owner string) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, s.query(`SELECT workspace_id FROM worker_subscriptions WHERE owner=? ORDER BY workspace_id`), owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
