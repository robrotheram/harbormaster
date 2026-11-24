package store

import (
	"context"
	"database/sql"
)

type SavedVisualization struct {
	WorkerID   string `json:"worker_id"`
	ArtifactID string `json:"artifact_id"`
	Title      string `json:"title"`
	Type       string `json:"type"`
}

type VisualizationDetails struct {
	WorkerID    string  `json:"worker_id"`
	ArtifactID  string  `json:"artifact_id"`
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
}

func migrateSavedVisualizations(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `CREATE TABLE saved_visualizations(
		project_id TEXT NOT NULL REFERENCES projects(id),
		owner TEXT NOT NULL,
		worker_id TEXT NOT NULL,
		artifact_id TEXT NOT NULL,
		title TEXT NOT NULL,
		type TEXT NOT NULL,
		saved_at BIGINT NOT NULL,
		PRIMARY KEY(project_id, owner, worker_id, artifact_id)
	)`)
	return err
}

func (s *Store) SavedVisualizations(ctx context.Context, project, owner string) ([]SavedVisualization, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT s.worker_id, s.artifact_id, COALESCE(NULLIF(t.title, ''), s.title), s.type
		FROM saved_visualizations s LEFT JOIN visualization_titles t USING(project_id, owner, worker_id, artifact_id)
		WHERE s.project_id=$1 AND s.owner=$2 ORDER BY s.saved_at DESC, s.worker_id, s.artifact_id`, project, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []SavedVisualization{}
	for rows.Next() {
		var item SavedVisualization
		if err := rows.Scan(&item.WorkerID, &item.ArtifactID, &item.Title, &item.Type); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) SaveVisualization(ctx context.Context, project, owner string, item SavedVisualization, at int64) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO saved_visualizations(project_id, owner, worker_id, artifact_id, title, type, saved_at)
		VALUES($1, $2, $3, $4, $5, $6, $7) ON CONFLICT(project_id, owner, worker_id, artifact_id) DO NOTHING`,
		project, owner, item.WorkerID, item.ArtifactID, item.Title, item.Type, at)
	return err
}

func (s *Store) RemoveSavedVisualization(ctx context.Context, project, owner, worker, artifact string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM saved_visualizations WHERE project_id=$1 AND owner=$2 AND worker_id=$3 AND artifact_id=$4`, project, owner, worker, artifact)
	return err
}

func migrateVisualizationTitles(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `CREATE TABLE visualization_titles(
		project_id TEXT NOT NULL REFERENCES projects(id),
		owner TEXT NOT NULL,
		worker_id TEXT NOT NULL,
		artifact_id TEXT NOT NULL,
		title TEXT NOT NULL,
		PRIMARY KEY(project_id, owner, worker_id, artifact_id)
	)`)
	return err
}

func migrateVisualizationDescriptions(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `ALTER TABLE visualization_titles ADD COLUMN description TEXT`)
	return err
}

func (s *Store) VisualizationEdits(ctx context.Context, project, owner string) ([]VisualizationDetails, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT worker_id, artifact_id, NULLIF(title, ''), description FROM visualization_titles
		WHERE project_id=$1 AND owner=$2`, project, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	edits := []VisualizationDetails{}
	for rows.Next() {
		var item VisualizationDetails
		if err := rows.Scan(&item.WorkerID, &item.ArtifactID, &item.Title, &item.Description); err != nil {
			return nil, err
		}
		edits = append(edits, item)
	}
	return edits, rows.Err()
}

func (s *Store) HasVisualizationReference(ctx context.Context, project, owner, worker, artifact string) (bool, error) {
	var known bool
	err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM saved_visualizations WHERE project_id=$1 AND owner=$2 AND worker_id=$3 AND artifact_id=$4
		UNION ALL
		SELECT 1 FROM visualization_titles WHERE project_id=$1 AND owner=$2 AND worker_id=$3 AND artifact_id=$4
	)`, project, owner, worker, artifact).Scan(&known)
	return known, err
}

func (s *Store) UpdateVisualizationDetails(ctx context.Context, project, owner string, item VisualizationDetails) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO visualization_titles(project_id, owner, worker_id, artifact_id, title, description)
		VALUES($1, $2, $3, $4, COALESCE($5, ''), $6)
		ON CONFLICT(project_id, owner, worker_id, artifact_id) DO UPDATE
		SET title=COALESCE($5, visualization_titles.title), description=COALESCE($6, visualization_titles.description)`,
		project, owner, item.WorkerID, item.ArtifactID, item.Title, item.Description)
	return err
}
