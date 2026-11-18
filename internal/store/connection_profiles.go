package store

import (
	"context"
	"encoding/json"
	"errors"

	"agent-workbench/internal/jsonmetadata"
	"agent-workbench/internal/shared"
)

var ErrProfileConflict = errors.New("profile revision conflict")
var ErrProfileInUse = errors.New("connection profile is in use")

func (s *Store) initConnectionProfiles(ctx context.Context) error {
	if _, err := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS connection_profiles(id TEXT PRIMARY KEY, revision INTEGER NOT NULL, metadata TEXT NOT NULL)`); err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS deleted_connection_profiles(id TEXT PRIMARY KEY)`); err != nil {
		return err
	}
	for _, p := range shared.DefaultConnectionProfiles() {
		b, _ := json.Marshal(p)
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO connection_profiles(id,revision,metadata) SELECT $1,$2,$3 WHERE NOT EXISTS (SELECT 1 FROM deleted_connection_profiles WHERE id=$4) ON CONFLICT(id) DO NOTHING`, p.ID, p.Revision, string(b), p.ID); err != nil {
			return err
		}
	}
	return nil
}
func (s *Store) ConnectionProfile(ctx context.Context, id string) (shared.ConnectionProfile, error) {
	return jsonmetadata.Get[shared.ConnectionProfile](ctx, s.DB, `SELECT metadata FROM connection_profiles WHERE id=$1`, id)
}
func (s *Store) ConnectionProfiles(ctx context.Context) ([]shared.ConnectionProfile, error) {
	return jsonmetadata.Query[shared.ConnectionProfile](ctx, s.DB, `SELECT metadata FROM connection_profiles ORDER BY id`)
}
func (s *Store) SaveConnectionProfile(ctx context.Context, p shared.ConnectionProfile, expected int) error {
	b, _ := json.Marshal(p)
	if expected == 0 {
		result, err := s.DB.ExecContext(ctx, `INSERT INTO connection_profiles(id,revision,metadata) VALUES($1,$2,$3) ON CONFLICT(id) DO NOTHING`, p.ID, p.Revision, string(b))
		if err != nil {
			return err
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			return ErrProfileConflict
		}
		return nil
	}
	result, err := s.DB.ExecContext(ctx, `UPDATE connection_profiles SET revision=$1,metadata=$2 WHERE id=$3 AND revision=$4`, p.Revision, string(b), p.ID, expected)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return ErrProfileConflict
	}
	return nil
}

// DeleteConnectionProfile keeps a tombstone so deleted seed profiles stay deleted.
func (s *Store) DeleteConnectionProfile(ctx context.Context, id string, expected int) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Both kinds of references must be moved before removing their profile.
	rows, err := tx.QueryContext(ctx, `SELECT metadata FROM workspaces UNION ALL SELECT metadata FROM hermes_setups`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var ref struct {
			ProfileID string `json:"connection_profile_id"`
		}
		if err = jsonmetadata.Scan(rows, &ref); err != nil {
			rows.Close()
			return err
		}
		if ref.ProfileID == id {
			rows.Close()
			return ErrProfileInUse
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM connection_profiles WHERE id=$1 AND revision=$2`, id, expected)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrProfileConflict
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO deleted_connection_profiles(id) VALUES($1) ON CONFLICT(id) DO NOTHING`, id); err != nil {
		return err
	}
	return tx.Commit()
}
