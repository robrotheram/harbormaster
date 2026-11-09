package store

import (
	"agent-workbench/internal/shared"
	"context"
	"encoding/json"
	"time"
)

func (s *Store) initToolProfiles(ctx context.Context) error {
	if _, err := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS tool_profiles(id TEXT PRIMARY KEY, revision INTEGER NOT NULL, metadata TEXT NOT NULL)`); err != nil {
		return err
	}
	for _, p := range shared.Profiles() {
		p.Revision = 1
		p.UpdatedAt = time.Now().UTC()
		b, _ := json.Marshal(p)
		if _, err := s.DB.ExecContext(ctx, s.query(`INSERT INTO tool_profiles(id,revision,metadata) VALUES(?,?,?) ON CONFLICT(id) DO NOTHING`), p.ID, p.Revision, string(b)); err != nil {
			return err
		}
	}
	return nil
}
func (s *Store) ToolProfile(ctx context.Context, id string) (shared.Profile, error) {
	var p shared.Profile
	var b string
	err := s.DB.QueryRowContext(ctx, s.query(`SELECT metadata FROM tool_profiles WHERE id=?`), id).Scan(&b)
	if err != nil {
		return p, err
	}
	err = json.Unmarshal([]byte(b), &p)
	return p, err
}
func (s *Store) ToolProfiles(ctx context.Context) ([]shared.Profile, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT metadata FROM tool_profiles ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []shared.Profile{}
	for rows.Next() {
		var p shared.Profile
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
func (s *Store) SaveToolProfile(ctx context.Context, p shared.Profile, expected int) error {
	b, _ := json.Marshal(p)
	if expected == 0 {
		result, err := s.DB.ExecContext(ctx, s.query(`INSERT INTO tool_profiles(id,revision,metadata) VALUES(?,?,?) ON CONFLICT(id) DO NOTHING`), p.ID, p.Revision, string(b))
		if err != nil {
			return err
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			return ErrProfileConflict
		}
		return nil
	}
	result, err := s.DB.ExecContext(ctx, s.query(`UPDATE tool_profiles SET revision=?,metadata=? WHERE id=? AND revision=?`), p.Revision, string(b), p.ID, expected)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return ErrProfileConflict
	}
	return nil
}
