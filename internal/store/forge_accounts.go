package store

import (
	"context"
	"encoding/json"
	"errors"

	"agent-workbench/internal/forge"
	"agent-workbench/internal/jsonmetadata"
)

var ErrForgeAccountConflict = errors.New("forge account conflict")

type ForgeAccount struct {
	forge.Host
	Revision int  `json:"revision"`
	Managed  bool `json:"managed"`
}

func (s *Store) ForgeAccounts(ctx context.Context) ([]ForgeAccount, error) {
	accounts, err := jsonmetadata.Query[ForgeAccount](ctx, s.DB, `SELECT metadata FROM forge_accounts ORDER BY name`)
	for i := range accounts {
		accounts[i].Token = ""
	}
	return accounts, err
}
func (s *Store) ForgeAccountCredential(ctx context.Context, name string) (string, error) {
	var value string
	err := s.DB.QueryRowContext(ctx, `SELECT credential FROM forge_accounts WHERE name=$1`, name).Scan(&value)
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
		res, err := s.DB.ExecContext(ctx, `INSERT INTO forge_accounts(name,revision,metadata,credential) VALUES($1,$2,$3,$4) ON CONFLICT(name) DO NOTHING`, a.Name, a.Revision, string(b), sealed)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return ErrForgeAccountConflict
		}
		return nil
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE forge_accounts SET revision=$1,metadata=$2,credential=$3 WHERE name=$4 AND revision=$5`, a.Revision, string(b), sealed, a.Name, expected)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return ErrForgeAccountConflict
	}
	return nil
}
