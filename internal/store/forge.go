package store

import (
	"context"
	"encoding/json"

	"agent-workbench/internal/jsonmetadata"
)

// ForgeConnection is a platform resource.
type ForgeConnection struct {
	ID         string `json:"id"`
	Host       string `json:"host"`
	Repository string `json:"repository"`
}

func (s *Store) ForgeConnections(ctx context.Context) ([]ForgeConnection, error) {
	return jsonmetadata.Query[ForgeConnection](ctx, s.DB, `SELECT metadata FROM forge_connections ORDER BY id`)
}
func (s *Store) SaveForge(ctx context.Context, c ForgeConnection) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO forge_connections(id,metadata) VALUES($1,$2)`, c.ID, string(b))
	return err
}
func (s *Store) DeleteForge(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM forge_connections WHERE id=$1`, id)
	return err
}
