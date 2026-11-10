package workspace

import (
	"agent-workbench/internal/shared"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"syscall"
	"time"
)

// Archive all available fixed recipe outputs, including diagnostics on failure.
// A successful job is not reported until immutable evidence is committed.
func (m *Manager) archiveRecipeEvidence(id, template string, success bool) error {
	names := []string{"execution.log"}
	if success {
		if template == "gogallery-v1" {
			names = append(names, "gallery.tar", "provenance.json")
		} else {
			names = append(names, "baseline.json", "variation.json")
		}
	}
	tx, err := m.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, name := range names {
		source := "outputs/" + id + "/" + name
		f, err := m.root.OpenFile(source, os.O_RDONLY|syscall.O_NONBLOCK, 0)
		if err != nil {
			return err
		}
		info, err := f.Stat()
		if err != nil {
			f.Close()
			return err
		}
		if !info.Mode().IsRegular() {
			f.Close()
			return os.ErrInvalid
		}
		b, err := io.ReadAll(io.LimitReader(f, (32<<20)+1))
		f.Close()
		if err != nil {
			return err
		}
		if len(b) > 32<<20 {
			return os.ErrInvalid
		}
		digest := sha256.Sum256(b)
		if _, err = tx.Exec(`INSERT INTO artifacts VALUES(?,?,?,?,?,?,?)`, shared.NewID("artifact_"), id+":"+name, source, hex.EncodeToString(digest[:]), len(b), time.Now().UTC().Format(time.RFC3339Nano), b); err != nil {
			return err
		}
	}
	return tx.Commit()
}
