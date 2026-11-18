package store

import (
	"context"
	"database/sql"
	"errors"
)

var ErrSubscriptionInUse = errors.New("SUBSCRIPTION_IN_USE")

type AgentSubscription struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Provider   string `json:"provider"`
	Credential string `json:"-"`
}

func (s *Store) AgentSubscription(ctx context.Context, owner, id string) (AgentSubscription, error) {
	var v AgentSubscription
	err := s.DB.QueryRowContext(ctx, s.query(`SELECT id,name,provider,credential FROM agent_subscriptions WHERE owner=? AND id=?`), owner, id).Scan(&v.ID, &v.Name, &v.Provider, &v.Credential)
	return v, err
}
func (s *Store) AgentSubscriptions(ctx context.Context, owner string) ([]AgentSubscription, error) {
	rows, err := s.DB.QueryContext(ctx, s.query(`SELECT id,name,provider,credential FROM agent_subscriptions WHERE owner=? ORDER BY name,id`), owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AgentSubscription{}
	for rows.Next() {
		var v AgentSubscription
		if err = rows.Scan(&v.ID, &v.Name, &v.Provider, &v.Credential); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) SaveAgentSubscription(ctx context.Context, owner string, v AgentSubscription, create bool) error {
	if create {
		_, err := s.DB.ExecContext(ctx, s.query(`INSERT INTO agent_subscriptions(owner,id,name,provider,credential) VALUES(?,?,?,?,?)`), owner, v.ID, v.Name, v.Provider, v.Credential)
		return err
	}
	res, err := s.DB.ExecContext(ctx, s.query(`UPDATE agent_subscriptions SET name=?,credential=? WHERE owner=? AND id=? AND provider=?`), v.Name, v.Credential, owner, v.ID, v.Provider)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return sql.ErrNoRows
	}
	return nil
}
func (s *Store) WorkerAgentSubscription(ctx context.Context, workspace, provider string) (string, string, error) {
	var owner, id string
	err := s.DB.QueryRowContext(ctx, s.query(`SELECT owner,subscription_id FROM worker_agent_subscriptions WHERE workspace_id=? AND provider=?`), workspace, provider).Scan(&owner, &id)
	return owner, id, err
}
func (s *Store) LoadAgentSubscription(ctx context.Context, owner, id, provider, workspace string) error {
	// The conditional upsert atomically prevents taking over another user's binding.
	res, err := s.DB.ExecContext(ctx, s.query(`INSERT INTO worker_agent_subscriptions(workspace_id,provider,owner,subscription_id) VALUES(?,?,?,?) ON CONFLICT(workspace_id,provider) DO UPDATE SET subscription_id=excluded.subscription_id WHERE worker_agent_subscriptions.owner=excluded.owner`), workspace, provider, owner, id)
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
func (s *Store) UnloadAgentSubscription(ctx context.Context, owner, provider, workspace string) error {
	_, err := s.DB.ExecContext(ctx, s.query(`DELETE FROM worker_agent_subscriptions WHERE workspace_id=? AND provider=? AND owner=?`), workspace, provider, owner)
	return err
}
func (s *Store) DeleteAgentSubscription(ctx context.Context, owner, id string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, s.query(`DELETE FROM worker_agent_subscriptions WHERE owner=? AND subscription_id=?`), owner, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, s.query(`DELETE FROM agent_subscriptions WHERE owner=? AND id=?`), owner, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}
func (s *Store) AgentSubscriptionWorkers(ctx context.Context, owner, id string) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, s.query(`SELECT workspace_id FROM worker_agent_subscriptions WHERE owner=? AND subscription_id=? ORDER BY workspace_id`), owner, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var workspace string
		if err = rows.Scan(&workspace); err != nil {
			return nil, err
		}
		out = append(out, workspace)
	}
	return out, rows.Err()
}

// Compatibility methods address only the migrated default Codex subscription.
func (s *Store) Subscription(ctx context.Context, owner string) (string, error) {
	v, e := s.AgentSubscription(ctx, owner, "default")
	return v.Credential, e
}
func (s *Store) SaveSubscription(ctx context.Context, owner, value string) error {
	v, e := s.AgentSubscription(ctx, owner, "default")
	create := errors.Is(e, sql.ErrNoRows)
	if e != nil && !create {
		return e
	}
	if create {
		v = AgentSubscription{ID: "default", Name: "Codex subscription", Provider: "codex"}
	}
	v.Credential = value
	return s.SaveAgentSubscription(ctx, owner, v, create)
}
func (s *Store) SubscriptionOwner(ctx context.Context, workspace string) (string, error) {
	owner, _, e := s.WorkerAgentSubscription(ctx, workspace, "codex")
	return owner, e
}
func (s *Store) LoadSubscription(ctx context.Context, owner, workspace string) error {
	return s.LoadAgentSubscription(ctx, owner, "default", "codex", workspace)
}
func (s *Store) UnloadSubscription(ctx context.Context, owner, workspace string) error {
	return s.UnloadAgentSubscription(ctx, owner, "codex", workspace)
}
func (s *Store) DeleteSubscription(ctx context.Context, owner string) error {
	return s.DeleteAgentSubscription(ctx, owner, "default")
}
func (s *Store) SubscriptionWorkers(ctx context.Context, owner string) ([]string, error) {
	return s.AgentSubscriptionWorkers(ctx, owner, "default")
}
