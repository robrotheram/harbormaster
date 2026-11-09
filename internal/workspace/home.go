package workspace

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"

	"agent-workbench/internal/shared"
)

type homeOwner struct {
	WorkspaceID string `json:"workspace_id"`
	ProjectID   string `json:"project_id"`
	HomeID      string `json:"home_id"`
}

func initializeHome(cfg Config) (*os.File, error) {
	if cfg.ProjectID == "" {
		cfg.ProjectID = "default"
	}
	if !shared.ValidID(cfg.ProjectID) || (cfg.HomeID != "" && !shared.ValidID(cfg.HomeID)) || (cfg.ProjectID != "default" && cfg.HomeID == "") {
		return nil, errors.New("invalid home ownership")
	}
	if err := os.MkdirAll(cfg.DataDir, 0700); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(filepath.Join(cfg.DataDir, ".workbench-owner.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, errors.New("workspace storage already owned by another supervisor")
	}
	fail := func(err error) (*os.File, error) { lock.Close(); return nil, err }
	owner := homeOwner{WorkspaceID: cfg.ID, ProjectID: cfg.ProjectID, HomeID: cfg.HomeID}
	name := filepath.Join(cfg.DataDir, ".workbench-owner.json")
	if info, err := os.Lstat(name); err == nil {
		if !info.Mode().IsRegular() {
			return fail(errors.New("invalid ownership record"))
		}
		b, err := os.ReadFile(name)
		var existing homeOwner
		if err != nil || json.Unmarshal(b, &existing) != nil || existing != owner {
			return fail(errors.New("persistent home ownership mismatch"))
		}
	} else if !os.IsNotExist(err) {
		return fail(err)
	} else {
		b, _ := json.Marshal(owner)
		if err = os.WriteFile(name, b, 0600); err != nil {
			return fail(err)
		}
	}
	return lock, nil
}

func (m *Manager) homePath() string {
	if m.cfg.HomeID == "" {
		return m.cfg.DataDir
	}
	return filepath.Join(m.cfg.DataDir, "homes", m.cfg.HomeID)
}
