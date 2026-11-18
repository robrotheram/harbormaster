package store

import (
	"context"
	"encoding/json"
)

// ForgeConnection is a platform resource.
type ForgeConnection struct {
	ID         string `json:"id"`
	Host       string `json:"host"`
	Repository string `json:"repository"`
}

func (s *Store) ForgeConnections(ctx context.Context) ([]ForgeConnection, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT metadata FROM forge_connections ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ForgeConnection{}
	for rows.Next() {
		var b string
		var item ForgeConnection
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(b), &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
func (s *Store) SaveForge(ctx context.Context, c ForgeConnection) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, s.query(`INSERT INTO forge_connections(id,metadata) VALUES(?,?)`), c.ID, string(b))
	return err
}
func (s *Store) DeleteForge(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, s.query(`DELETE FROM forge_connections WHERE id=?`), id)
	return err
}
