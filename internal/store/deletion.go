package store

import (
	"agent-workbench/internal/shared"
	"context"
	"net/http"
)

// Tombstones preserve ownership and delivery keys. Physical private disks are
// retained: the runtime contract has no portable purge operation.
func (s *Store) DeleteStopped(ctx context.Context, workspaces []shared.Workspace, project string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, ws := range workspaces {
		if ws.State != "STOPPED" {
			return shared.CodedError{Status: http.StatusConflict, Code: "STOP_WORKSPACE_FIRST"}
		}
		if _, err = tx.ExecContext(ctx, s.query(`INSERT INTO deleted_resources(kind,id) VALUES('workspace',?) ON CONFLICT(kind,id) DO NOTHING`), ws.ID); err != nil {
			return err
		}
	}
	if project != "" {
		if _, err = tx.ExecContext(ctx, s.query(`INSERT INTO deleted_resources(kind,id) VALUES('project',?) ON CONFLICT(kind,id) DO NOTHING`), project); err != nil {
			return err
		}
	}
	return tx.Commit()
}
