package store

import (
	"agent-workbench/internal/forge"
	"context"
	"encoding/json"
	"errors"
)

var ErrForgeAccountConflict = errors.New("forge account conflict")

type ForgeAccount struct {
	forge.Host
	Revision int  `json:"revision"`
	Managed  bool `json:"managed"`
}

func (s *Store) initForgeAccounts(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS forge_accounts(name TEXT PRIMARY KEY, revision INTEGER NOT NULL, metadata TEXT NOT NULL, credential TEXT NOT NULL)`)
	return err
}
func (s *Store) ForgeAccounts(ctx context.Context) ([]ForgeAccount, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT metadata FROM forge_accounts ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	accounts := []ForgeAccount{}
	for rows.Next() {
		var b string
		var a ForgeAccount
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(b), &a); err != nil {
			return nil, err
		}
		a.Token = ""
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}
func (s *Store) ForgeAccountCredential(ctx context.Context, name string) (string, error) {
	var value string
	err := s.DB.QueryRowContext(ctx, s.query(`SELECT credential FROM forge_accounts WHERE name=?`), name).Scan(&value)
	return value, err
}
func (s *Store) SaveForgeAccount(ctx context.Context, a ForgeAccount, sealed string, expected int) error {
	a.Token = ""
	a.Managed = true
	a.Revision = expected + 1
	b, err := json.Marshal(a)
	if err != nil {
		return err
	}
	if expected == 0 {
		res, err := s.DB.ExecContext(ctx, s.query(`INSERT INTO forge_accounts(name,revision,metadata,credential) VALUES(?,?,?,?) ON CONFLICT(name) DO NOTHING`), a.Name, a.Revision, string(b), sealed)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return ErrForgeAccountConflict
		}
		return nil
	}
	res, err := s.DB.ExecContext(ctx, s.query(`UPDATE forge_accounts SET revision=?,metadata=?,credential=? WHERE name=? AND revision=?`), a.Revision, string(b), sealed, a.Name, expected)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return ErrForgeAccountConflict
	}
	return nil
}
