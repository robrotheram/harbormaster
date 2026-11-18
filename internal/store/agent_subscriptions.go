package store

import (
	"context"
	"database/sql"
)

type AgentSubscription struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Provider   string `json:"provider"`
	Credential string `json:"-"`
}

func (s *Store) initAgentSubscriptions(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS agent_subscriptions(owner TEXT NOT NULL,id TEXT NOT NULL,name TEXT NOT NULL,provider TEXT NOT NULL,credential TEXT NOT NULL,PRIMARY KEY(owner,id))`,
		`INSERT INTO agent_subscriptions(owner,id,name,provider,credential) SELECT owner,'default','Codex subscription','codex',credential FROM user_subscriptions WHERE true ON CONFLICT(owner,id) DO NOTHING`,
		`CREATE TABLE IF NOT EXISTS worker_agent_subscriptions(workspace_id TEXT NOT NULL,provider TEXT NOT NULL,owner TEXT NOT NULL,subscription_id TEXT NOT NULL,PRIMARY KEY(workspace_id,provider))`,
		`INSERT INTO worker_agent_subscriptions(workspace_id,provider,owner,subscription_id) SELECT workspace_id,'codex',owner,'default' FROM worker_subscriptions WHERE true ON CONFLICT(workspace_id,provider) DO NOTHING`,
		`DROP TABLE worker_subscriptions`, `DROP TABLE user_subscriptions`,
		// Commit the version marker with the destructive migration so a restart cannot replay it.
		`INSERT INTO schema_migrations(version) VALUES(14) ON CONFLICT(version) DO NOTHING`,
	} {
		if _, err = tx.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *Store) AgentSubscription(ctx context.Context, owner, id string) (AgentSubscription, error) {
	var v AgentSubscription
	err := s.DB.QueryRowContext(ctx, `SELECT id,name,provider,credential FROM agent_subscriptions WHERE owner=$1 AND id=$2`, owner, id).Scan(&v.ID, &v.Name, &v.Provider, &v.Credential)
	return v, err
}
func (s *Store) AgentSubscriptions(ctx context.Context, owner string) ([]AgentSubscription, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,name,provider,credential FROM agent_subscriptions WHERE owner=$1 ORDER BY name,id`, owner)
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
		_, err := s.DB.ExecContext(ctx, `INSERT INTO agent_subscriptions(owner,id,name,provider,credential) VALUES($1,$2,$3,$4,$5)`, owner, v.ID, v.Name, v.Provider, v.Credential)
		return err
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE agent_subscriptions SET name=$1,credential=$2 WHERE owner=$3 AND id=$4 AND provider=$5`, v.Name, v.Credential, owner, v.ID, v.Provider)
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
	err := s.DB.QueryRowContext(ctx, `SELECT owner,subscription_id FROM worker_agent_subscriptions WHERE workspace_id=$1 AND provider=$2`, workspace, provider).Scan(&owner, &id)
	return owner, id, err
}
func (s *Store) LoadAgentSubscription(ctx context.Context, owner, id, provider, workspace string) error {
	// The conditional upsert atomically prevents taking over another user's binding.
	res, err := s.DB.ExecContext(ctx, `INSERT INTO worker_agent_subscriptions(workspace_id,provider,owner,subscription_id) VALUES($1,$2,$3,$4) ON CONFLICT(workspace_id,provider) DO UPDATE SET subscription_id=excluded.subscription_id WHERE worker_agent_subscriptions.owner=excluded.owner`, workspace, provider, owner, id)
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
	_, err := s.DB.ExecContext(ctx, `DELETE FROM worker_agent_subscriptions WHERE workspace_id=$1 AND provider=$2 AND owner=$3`, workspace, provider, owner)
	return err
}
func (s *Store) DeleteAgentSubscription(ctx context.Context, owner, id string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM worker_agent_subscriptions WHERE owner=$1 AND subscription_id=$2`, owner, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM agent_subscriptions WHERE owner=$1 AND id=$2`, owner, id)
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
	rows, err := s.DB.QueryContext(ctx, `SELECT workspace_id FROM worker_agent_subscriptions WHERE owner=$1 AND subscription_id=$2 ORDER BY workspace_id`, owner, id)
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
