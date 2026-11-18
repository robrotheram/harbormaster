package store

import (
	"context"
	"database/sql"
	"errors"
)

type ProjectMember struct {
	ProjectID string `json:"project_id"`
	Email     string `json:"email"`
	UserID    string `json:"user_id"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	Revision  int    `json:"revision"`
}

var ErrMembershipConflict = errors.New("membership changed or last admin")

func (s *Store) ProjectMembers(ctx context.Context, project string) ([]ProjectMember, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT project_id,email,user_id,role,status,revision FROM project_members WHERE project_id=$1 ORDER BY email`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProjectMember{}
	for rows.Next() {
		var m ProjectMember
		if err = rows.Scan(&m.ProjectID, &m.Email, &m.UserID, &m.Role, &m.Status, &m.Revision); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func (s *Store) MemberRole(ctx context.Context, project, user string) (string, error) {
	var role string
	err := s.DB.QueryRowContext(ctx, `SELECT role FROM project_members WHERE project_id=$1 AND user_id=$2 AND status='active'`, project, user).Scan(&role)
	return role, err
}
func (s *Store) Invitations(ctx context.Context, email string) ([]ProjectMember, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT project_id,email,user_id,role,status,revision FROM project_members WHERE email=$1 AND status='invited' AND project_id NOT IN (SELECT id FROM deleted_resources WHERE kind='project')`, email)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProjectMember{}
	for rows.Next() {
		var m ProjectMember
		if err = rows.Scan(&m.ProjectID, &m.Email, &m.UserID, &m.Role, &m.Status, &m.Revision); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// The write lock on the project serializes roster changes across API instances.
func (s *Store) SaveMember(ctx context.Context, m ProjectMember, expected int, actor string, system bool) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE projects SET metadata=metadata WHERE id=$1 AND id NOT IN (SELECT id FROM deleted_resources WHERE kind='project')`, m.ProjectID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return sql.ErrNoRows
	}
	if !system {
		var role string
		err = tx.QueryRowContext(ctx, `SELECT role FROM project_members WHERE project_id=$1 AND user_id=$2 AND status='active'`, m.ProjectID, actor).Scan(&role)
		if err != nil || role != "admin" {
			return sql.ErrNoRows
		}
	}
	var oldRole, oldStatus string
	var oldRev int
	err = tx.QueryRowContext(ctx, `SELECT role,status,revision FROM project_members WHERE project_id=$1 AND email=$2`, m.ProjectID, m.Email).Scan(&oldRole, &oldStatus, &oldRev)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if oldRev != expected {
		return ErrMembershipConflict
	}
	if oldRole == "admin" && oldStatus == "active" && (m.Role != "admin" || m.Status != "active") {
		var count int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM project_members WHERE project_id=$1 AND role='admin' AND status='active'`, m.ProjectID).Scan(&count); err != nil {
			return err
		}
		if count <= 1 {
			return ErrMembershipConflict
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO project_members(project_id,email,user_id,role,status,revision) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(project_id,email) DO UPDATE SET user_id=excluded.user_id,role=excluded.role,status=excluded.status,revision=excluded.revision`, m.ProjectID, m.Email, m.UserID, m.Role, m.Status, expected+1)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) AcceptInvitation(ctx context.Context, project, email, user string, revision int) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Use the same project lock as roster edits. Acceptance must not race past
	// the last-admin check or overwrite a concurrent revocation.
	result, err := tx.ExecContext(ctx, `UPDATE projects SET metadata=metadata WHERE id=$1 AND id NOT IN (SELECT id FROM deleted_resources WHERE kind='project')`, project)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return ErrMembershipConflict
	}
	result, err = tx.ExecContext(ctx, `UPDATE project_members SET user_id=$1,status='active',revision=revision+1 WHERE project_id=$2 AND email=$3 AND status='invited' AND revision=$4`, user, project, email, revision)
	if err != nil {
		return err
	}
	n, _ = result.RowsAffected()
	if n != 1 {
		return ErrMembershipConflict
	}
	return tx.Commit()
}
