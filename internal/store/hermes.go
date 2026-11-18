package store

import (
	"context"
	"encoding/json"
	"errors"

	"agent-workbench/internal/jsonmetadata"
	"agent-workbench/internal/shared"
)

var ErrHermesRevision = errors.New("Hermes configuration revision conflict")

func (s *Store) initHermes(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS hermes_setups(project_id TEXT PRIMARY KEY REFERENCES projects(id),revision INTEGER NOT NULL,metadata TEXT NOT NULL)`)
	return err
}
func (s *Store) HermesSetup(ctx context.Context, project string) (shared.HermesSetup, error) {
	return jsonmetadata.Get[shared.HermesSetup](ctx, s.DB, `SELECT metadata FROM hermes_setups WHERE project_id=$1`, project)
}
func (s *Store) SaveHermesSetup(ctx context.Context, c shared.HermesSetup, previous int) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	q := `UPDATE hermes_setups SET revision=$1,metadata=$2 WHERE project_id=$3 AND revision=$4`
	args := []any{c.Revision, string(b), c.ProjectID, previous}
	if previous == 0 {
		q = `INSERT INTO hermes_setups(project_id,revision,metadata) VALUES($1,$2,$3) ON CONFLICT(project_id) DO NOTHING`
		args = []any{c.ProjectID, c.Revision, string(b)}
	}
	r, err := s.DB.ExecContext(ctx, q, args...)
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
