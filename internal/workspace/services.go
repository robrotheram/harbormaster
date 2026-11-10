package workspace

// Retained previews use immutable, validated snapshots. This wire API intentionally
// returns JSON bytes, never active HTML on the supervisor/operator origin.
import (
	"agent-workbench/internal/shared"
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

type retainedService struct {
	ID        string `json:"id"`
	State     string `json:"state"`
	ExpiresAt int64  `json:"expires_at"`
}

func (m *Manager) service(id string) (retainedService, error) {
	s := retainedService{ID: id}
	err := m.db.QueryRow(`SELECT state,expires_at FROM retained_services WHERE id=?`, id).Scan(&s.State, &s.ExpiresAt)
	if err == nil && s.State == "active" && s.ExpiresAt <= time.Now().Unix() {
		_, err = m.db.Exec(`UPDATE retained_services SET state='expired' WHERE id=? AND state='active'`, id)
		s.State = "expired"
	}
	return s, err
}
func (m *Manager) createService(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Snapshot []byte `json:"snapshot"`
		Key      string `json:"idempotency_key"`
		Source   string `json:"source_ref"`
		TTL      int    `json:"ttl_seconds"`
	}
	if !shared.Decode(w, r, &in, 24<<20) {
		return
	}
	if len(in.Source) > 512 || len(in.Snapshot) > 16<<20 || in.TTL < 1 || in.TTL > 86400 {
		shared.Error(w, 400, "INVALID_PREVIEW_BUDGET")
		return
	}
	files := map[string][]byte{}
	total := 0
	tr := tar.NewReader(bytes.NewReader(in.Snapshot))
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			shared.Error(w, 400, "INVALID_SNAPSHOT")
			return
		}
		name := strings.TrimPrefix(h.Name, "site/")
		if h.Typeflag == tar.TypeDir {
			continue
		}
		if h.Typeflag != tar.TypeReg || !shared.ValidProjectFile(name) || path.Clean(name) != name || h.Size < 0 || h.Size > 2<<20 || len(files) >= 512 {
			shared.Error(w, 400, "INVALID_SNAPSHOT")
			return
		}
		b, err := io.ReadAll(io.LimitReader(tr, (2<<20)+1))
		total += len(b)
		if err != nil || len(b) > 2<<20 || total > 16<<20 {
			shared.Error(w, 400, "INVALID_SNAPSHOT")
			return
		}
		if _, exists := files[name]; exists {
			shared.Error(w, 400, "INVALID_SNAPSHOT")
			return
		}
		files[name] = b
	}
	if len(files["index.html"]) == 0 {
		shared.Error(w, 400, "MISSING_INDEX")
		return
	}
	snapshot, err := json.Marshal(files)
	if err != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.quiescing {
		shared.Error(w, 409, "WORKSPACE_STOPPING")
		return
	}
	fingerprint := fmt.Sprintf("%x:%d:%s", sha256.Sum256(in.Snapshot), in.TTL, in.Source)
	if in.Key != "" {
		if !shared.ValidKey(in.Key) {
			shared.Error(w, 400, "INVALID_RECEIPT_KEY")
			return
		}
		var prior, id string
		err := m.db.QueryRow(`SELECT fingerprint,service_id FROM retained_service_receipts WHERE idem_key=?`, in.Key).Scan(&prior, &id)
		if err == nil {
			if prior != fingerprint {
				shared.Error(w, 409, "IDEMPOTENCY_CONFLICT")
				return
			}
			service, err := m.service(id)
			if err != nil {
				shared.Error(w, 503, "STORAGE_UNAVAILABLE")
				return
			}
			shared.JSON(w, 201, service)
			return
		}
		if !errors.Is(err, sql.ErrNoRows) {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
	}
	var active int
	if err := m.db.QueryRow(`SELECT count(*) FROM retained_services WHERE state='active' AND expires_at>?`, time.Now().Unix()).Scan(&active); err != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	if active != 0 {
		shared.Error(w, 409, "ACTIVE_PREVIEW_EXISTS")
		return
	}
	s := retainedService{shared.NewID("service_"), "active", time.Now().Unix() + int64(in.TTL)}
	tx, err := m.db.Begin()
	if err != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT INTO retained_services VALUES(?,?,?,?)`, s.ID, s.State, s.ExpiresAt, snapshot); err == nil && in.Key != "" {
		_, err = tx.Exec(`INSERT INTO retained_service_receipts VALUES(?,?,?,?,?)`, in.Key, fingerprint, s.ID, in.Source, in.TTL)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	shared.JSON(w, 201, s)
}
func (m *Manager) getService(w http.ResponseWriter, r *http.Request) {
	s, err := m.service(r.PathValue("id"))
	if err != nil {
		shared.Error(w, 404, "SERVICE_NOT_FOUND")
		return
	}
	shared.JSON(w, 200, s)
}
func (m *Manager) revokeService(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.service(r.PathValue("id")); err != nil {
		shared.Error(w, 404, "SERVICE_NOT_FOUND")
		return
	}
	if _, err := m.db.Exec(`UPDATE retained_services SET state='revoked' WHERE id=?`, r.PathValue("id")); err != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	m.getService(w, r)
}
func (m *Manager) reconcileServices(w http.ResponseWriter, r *http.Request) {
	if _, err := m.db.Exec(`UPDATE retained_services SET state='expired' WHERE state='active' AND expires_at<=?`, time.Now().Unix()); err != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	shared.JSON(w, 200, map[string]bool{"reconciled": true})
}
func (m *Manager) fetchService(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Path string `json:"path"`
	}
	if !shared.Decode(w, r, &in, 2048) {
		return
	}
	if !shared.ValidProjectFile(in.Path) {
		shared.Error(w, 400, "INVALID_PATH")
		return
	}
	// Serialize access and revocation to avoid handing out content after acknowledgement.
	m.mu.Lock()
	defer m.mu.Unlock()
	s, err := m.service(r.PathValue("id"))
	if err != nil {
		shared.Error(w, 404, "SERVICE_NOT_FOUND")
		return
	}
	if s.State != "active" {
		shared.Error(w, 410, "SERVICE_REVOKED")
		return
	}
	var snapshot []byte
	if err = m.db.QueryRow(`SELECT snapshot FROM retained_services WHERE id=?`, s.ID).Scan(&snapshot); err != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	var files map[string][]byte
	if json.Unmarshal(snapshot, &files) != nil {
		shared.Error(w, 503, "INVALID_SNAPSHOT")
		return
	}
	b, ok := files[in.Path]
	if !ok {
		shared.Error(w, 404, "FILE_NOT_FOUND")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	shared.JSON(w, 200, map[string]any{"content": b, "content_type": mime.TypeByExtension(path.Ext(in.Path)), "attachment": "isolated-origin-required", "websocket": false})
}

func (m *Manager) expireServices() {
	defer m.wg.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.historyStop:
			return
		case <-ticker.C:
			m.db.Exec(`UPDATE retained_services SET state='expired' WHERE state='active' AND expires_at<=?`, time.Now().Unix())
		}
	}
}

// Receipt lookup lets a caller recover a lost acknowledgement even after source
// compute stopped. source_ref is the trusted platform immutable artifact binding.
func (m *Manager) serviceReceipt(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Key    string `json:"idempotency_key"`
		Source string `json:"source_ref"`
		TTL    int    `json:"ttl_seconds"`
	}
	if !shared.Decode(w, r, &in, 2048) {
		return
	}
	if !shared.ValidKey(in.Key) {
		shared.Error(w, 400, "INVALID_REQUEST")
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var id, source string
	var ttl int
	err := m.db.QueryRow(`SELECT service_id,source_ref,ttl FROM retained_service_receipts WHERE idem_key=?`, in.Key).Scan(&id, &source, &ttl)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.Error(w, 404, "RECEIPT_NOT_FOUND")
		return
	}
	if source != in.Source || ttl != in.TTL {
		shared.Error(w, 409, "IDEMPOTENCY_CONFLICT")
		return
	}
	service, err := m.service(id)
	if err != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	shared.JSON(w, 200, service)
}
