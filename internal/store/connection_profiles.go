package store

import (
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"errors"
)

var ErrProfileConflict = errors.New("profile revision conflict")

func (s *Store) initConnectionProfiles(ctx context.Context) error {
	if _, err := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS connection_profiles(id TEXT PRIMARY KEY, revision INTEGER NOT NULL, metadata TEXT NOT NULL)`); err != nil {
		return err
	}
	for _, p := range shared.DefaultConnectionProfiles() {
		b, _ := json.Marshal(p)
		if _, err := s.DB.ExecContext(ctx, s.query(`INSERT INTO connection_profiles(id,revision,metadata) VALUES(?,?,?) ON CONFLICT(id) DO NOTHING`), p.ID, p.Revision, string(b)); err != nil {
			return err
		}
	}
	return nil
}
func (s *Store) ConnectionProfile(ctx context.Context, id string) (shared.ConnectionProfile, error) {
	var p shared.ConnectionProfile
	var b string
	err := s.DB.QueryRowContext(ctx, s.query(`SELECT metadata FROM connection_profiles WHERE id=?`), id).Scan(&b)
	if err != nil {
		return p, err
	}
	err = json.Unmarshal([]byte(b), &p)
	return p, err
}
func (s *Store) ConnectionProfiles(ctx context.Context) ([]shared.ConnectionProfile, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT metadata FROM connection_profiles ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []shared.ConnectionProfile{}
	for rows.Next() {
		var p shared.ConnectionProfile
		var b string
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(b), &p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func (s *Store) SaveConnectionProfile(ctx context.Context, p shared.ConnectionProfile, expected int) error {
	b, _ := json.Marshal(p)
	if expected == 0 {
		result, err := s.DB.ExecContext(ctx, s.query(`INSERT INTO connection_profiles(id,revision,metadata) VALUES(?,?,?) ON CONFLICT(id) DO NOTHING`), p.ID, p.Revision, string(b))
		if err != nil {
			return err
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			return ErrProfileConflict
		}
		return nil
	}
	result, err := s.DB.ExecContext(ctx, s.query(`UPDATE connection_profiles SET revision=?,metadata=? WHERE id=? AND revision=?`), p.Revision, string(b), p.ID, expected)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return ErrProfileConflict
	}
	return nil
}
