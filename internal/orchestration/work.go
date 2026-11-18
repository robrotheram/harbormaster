package orchestration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"agent-workbench/internal/shared"
)

var ErrInvalidWork = errors.New("INVALID_REQUEST")

// CreateWork durably creates a draft. Acceptance/execution is deliberately a
// separate operation: saving a reference does not prove its bytes were verified.
func (r Repository) CreateWork(ctx context.Context, item shared.WorkItem, key string) (shared.WorkItem, bool, error) {
	if !shared.ValidWorkSelection(item.Provider, item.Model) || !shared.ValidID(item.ProjectID) || !shared.ValidID(item.WorkspaceID) || !shared.ValidKey(key) || !shared.ValidProjectFile(item.SpecificationPath) || len(item.SpecificationDigest) != 64 || item.SpecificationDigest != strings.ToLower(item.SpecificationDigest) {
		return shared.WorkItem{}, false, ErrInvalidWork
	}
	if _, err := hex.DecodeString(item.SpecificationDigest); err != nil {
		return shared.WorkItem{}, false, ErrInvalidWork
	}
	for _, path := range item.ArtifactPaths {
		if !shared.ValidProjectFile(path) {
			return shared.WorkItem{}, false, ErrInvalidWork
		}
	}
	if len(item.ArtifactPaths) > 32 {
		return shared.WorkItem{}, false, ErrInvalidWork
	}
	switch item.Kind {
	case "software", "research", "digest":
	default:
		return shared.WorkItem{}, false, ErrInvalidWork
	}
	hash := sha256.Sum256([]byte(item.ProjectID + "\x00" + key))
	item.ID = "work_" + hex.EncodeToString(hash[:16])
	item.State = "draft"
	item.CreatedAt = item.CreatedAt.UTC()
	item.UpdatedAt = item.CreatedAt
	body, err := json.Marshal(item)
	if err != nil {
		return item, false, err
	}
	result, err := r.DB.ExecContext(ctx, `INSERT INTO work_items(id,project_id,create_key,metadata) VALUES($1,$2,$3,$4) ON CONFLICT(project_id,create_key) DO NOTHING`, item.ID, item.ProjectID, key, string(body))
	if err != nil {
		return item, false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return item, false, err
	}
	stored, err := r.Work(ctx, item.ProjectID, item.ID)
	if err == nil && (stored.Provider != item.Provider || stored.Model != item.Model || stored.WorkspaceID != item.WorkspaceID || stored.SpecificationPath != item.SpecificationPath || stored.SpecificationDigest != item.SpecificationDigest || stored.Kind != item.Kind || !sameArtifactPaths(stored.ArtifactPaths, item.ArtifactPaths)) {
		err = ErrConflict
	}
	return stored, n == 1, err
}

func (r Repository) Work(ctx context.Context, project, id string) (shared.WorkItem, error) {
	var body string
	var item shared.WorkItem
	err := r.DB.QueryRowContext(ctx, `SELECT metadata FROM work_items WHERE project_id=$1 AND id=$2`, project, id).Scan(&body)
	if err == nil {
		err = json.Unmarshal([]byte(body), &item)
	}
	return item, err
}

func (r Repository) WorkItems(ctx context.Context, project string) ([]shared.WorkItem, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT metadata FROM work_items WHERE project_id=$1 ORDER BY id LIMIT 200`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []shared.WorkItem{}
	for rows.Next() {
		var body string
		var item shared.WorkItem
		if err = rows.Scan(&body); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(body), &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// CancelDraft uses compare-and-swap so a stale cancellation cannot overwrite
// a newer state. Repeated cancellation returns the existing cancelled record.
func (r Repository) CancelDraft(ctx context.Context, project, id string, now time.Time) (shared.WorkItem, error) {
	item, err := r.Work(ctx, project, id)
	if err != nil {
		return item, err
	}
	if item.State == "cancelled" {
		return item, nil
	}
	if item.State != "draft" {
		return item, ErrConflict
	}
	old, _ := json.Marshal(item)
	item.State = "cancelled"
	item.UpdatedAt = now.UTC()
	body, _ := json.Marshal(item)
	result, err := r.DB.ExecContext(ctx, `UPDATE work_items SET metadata=$1 WHERE project_id=$2 AND id=$3 AND metadata=$4`, string(body), project, id, string(old))
	if err != nil {
		return item, err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		err = ErrConflict
	}
	return item, err
}

func sameArtifactPaths(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ClaimWorkRequest retains only a fingerprint, allowing partial provisioning to
// reconcile without accepting a different request under the same delivery key.
func (r Repository) ClaimWorkRequest(ctx context.Context, project, key, fingerprint string) error {
	_, err := r.DB.ExecContext(ctx, `INSERT INTO work_create_requests(project_id,create_key,fingerprint) VALUES($1,$2,$3) ON CONFLICT(project_id,create_key) DO NOTHING`, project, key, fingerprint)
	if err != nil {
		return err
	}
	var prior string
	err = r.DB.QueryRowContext(ctx, `SELECT fingerprint FROM work_create_requests WHERE project_id=$1 AND create_key=$2`, project, key).Scan(&prior)
	if err == nil && prior != fingerprint {
		return ErrConflict
	}
	return err
}
