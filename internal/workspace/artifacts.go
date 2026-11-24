package workspace

import (
	"agent-workbench/internal/shared"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"io"
	"net/http"
	"time"
)

type Artifact struct {
	ID         string `json:"id"`
	SourcePath string `json:"source_path"`
	SHA256     string `json:"sha256"`
	SizeBytes  int64  `json:"size_bytes"`
	CreatedAt  string `json:"created_at"`
}

func (m *Manager) publishArtifact(w http.ResponseWriter, r *http.Request) {
	var input struct {
		SourcePath string `json:"source_path"`
		Key        string `json:"idempotency_key"`
	}
	if !shared.Decode(w, r, &input, 2048) {
		return
	}
	if !shared.ValidProjectFile(input.SourcePath) || !shared.ValidKey(input.Key) {
		shared.Error(w, 400, "INVALID_REQUEST")
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var a Artifact
	err := m.db.QueryRow(`SELECT id,source_path,sha256,size_bytes,created_at FROM artifacts WHERE idem_key=?`, input.Key).Scan(&a.ID, &a.SourcePath, &a.SHA256, &a.SizeBytes, &a.CreatedAt)
	if err == nil {
		if a.SourcePath != input.SourcePath {
			shared.Error(w, 409, "IDEMPOTENCY_CONFLICT")
			return
		}
		shared.JSON(w, 200, a)
		return
	}
	if err != sql.ErrNoRows {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	if m.closed || m.quiescing {
		shared.Error(w, 409, "WORKSPACE_STOPPING")
		return
	}
	f, err := openReviewFile(m.root, input.SourcePath)
	if err != nil {
		shared.Error(w, 400, "INVALID_ARTIFACT_SOURCE")
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 32<<20 {
		shared.Error(w, 400, "INVALID_ARTIFACT_SOURCE")
		return
	}
	b, err := io.ReadAll(io.LimitReader(f, (32<<20)+1))
	if err != nil || len(b) > 32<<20 {
		shared.Error(w, 400, "INVALID_ARTIFACT_SOURCE")
		return
	}
	sum := sha256.Sum256(b)
	a = Artifact{shared.NewID("artifact_"), input.SourcePath, hex.EncodeToString(sum[:]), int64(len(b)), time.Now().UTC().Format(time.RFC3339Nano)}
	if _, err = m.db.Exec(`INSERT INTO artifacts VALUES(?,?,?,?,?,?,?)`, a.ID, input.Key, a.SourcePath, a.SHA256, a.SizeBytes, a.CreatedAt, b); err != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	shared.JSON(w, 201, a)
}
func (m *Manager) listArtifacts(w http.ResponseWriter, r *http.Request) {
	rows, err := m.db.Query(`SELECT id,source_path,sha256,size_bytes,created_at FROM artifacts ORDER BY created_at,id LIMIT 200`)
	if err != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	defer rows.Close()
	items := []Artifact{}
	for rows.Next() {
		var a Artifact
		if err = rows.Scan(&a.ID, &a.SourcePath, &a.SHA256, &a.SizeBytes, &a.CreatedAt); err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		items = append(items, a)
	}
	if rows.Err() != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	shared.JSON(w, 200, map[string]any{"items": items})
}
func (m *Manager) artifactContent(w http.ResponseWriter, r *http.Request) {
	var b []byte
	if err := m.db.QueryRow(`SELECT content FROM artifacts WHERE id=?`, r.PathValue("id")).Scan(&b); err != nil {
		shared.Error(w, 404, "ARTIFACT_NOT_FOUND")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write(b)
}
