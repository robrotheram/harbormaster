package store

import (
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"errors"
)

var ErrHermesRevision = errors.New("Hermes configuration revision conflict")

func (s *Store) initHermes(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS hermes_setups(project_id TEXT PRIMARY KEY REFERENCES projects(id),revision INTEGER NOT NULL,metadata TEXT NOT NULL)`)
	return err
}
func (s *Store) HermesSetup(ctx context.Context, project string) (shared.HermesSetup, error) {
	var c shared.HermesSetup
	var b string
	err := s.DB.QueryRowContext(ctx, s.query(`SELECT metadata FROM hermes_setups WHERE project_id=?`), project).Scan(&b)
	if err != nil {
		return c, err
	}
	err = json.Unmarshal([]byte(b), &c)
	return c, err
}
func (s *Store) SaveHermesSetup(ctx context.Context, c shared.HermesSetup, previous int) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	q := `UPDATE hermes_setups SET revision=?,metadata=? WHERE project_id=? AND revision=?`
	args := []any{c.Revision, string(b), c.ProjectID, previous}
	if previous == 0 {
		q = `INSERT INTO hermes_setups(project_id,revision,metadata) VALUES(?,?,?) ON CONFLICT(project_id) DO NOTHING`
		args = []any{c.ProjectID, c.Revision, string(b)}
	}
	r, err := s.DB.ExecContext(ctx, s.query(q), args...)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrHermesRevision
	}
	return nil
}
