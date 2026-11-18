package store

import (
	"context"
	"database/sql"
	"errors"
)

// References are permanent within a project and never derived from list order.
func (s *Store) workReference(ctx context.Context, project, id string) (int, error) {
	var number int
	err := s.DB.QueryRowContext(ctx, `SELECT number FROM work_references WHERE project_id=$1 AND work_id=$2`, project, id).Scan(&number)
	if !errors.Is(err, sql.ErrNoRows) {
		return number, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	// Serialize allocation across API processes, including PostgreSQL.
	if _, err = tx.ExecContext(ctx, `UPDATE projects SET metadata=metadata WHERE id=$1`, project); err != nil {
		return 0, err
	}
	err = tx.QueryRowContext(ctx, `SELECT number FROM work_references WHERE project_id=$1 AND work_id=$2`, project, id).Scan(&number)
	if errors.Is(err, sql.ErrNoRows) {
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(number),0)+1 FROM work_references WHERE project_id=$1`, project).Scan(&number); err != nil {
			return 0, err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO work_references(project_id,work_id,number) VALUES($1,$2,$3)`, project, id, number)
	}
	if err != nil {
		return 0, err
	}
	return number, tx.Commit()
}
