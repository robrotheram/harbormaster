package workspace

import (
	"agent-workbench/internal/workspace/taskstore"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"os"
	"syscall"
	"unicode/utf8"

	"agent-workbench/internal/shared"
)

func (m *Manager) submitFile(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Path     string `json:"instruction_path"`
		Provider string `json:"provider"`
		Key      string `json:"idempotency_key"`
	}
	if !shared.Decode(w, r, &input, 2048) {
		return
	}
	if !shared.ValidProjectFile(input.Path) || !shared.ValidKey(input.Key) || (input.Provider != "" && input.Provider != "codex" && input.Provider != "claude") {
		shared.Error(w, 400, "INVALID_REQUEST")
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if input.Provider == "" {
		input.Provider = m.cfg.DefaultProvider
	}
	task, err := m.tasks.FileAcceptance(r.Context(), input.Key, input.Path, input.Provider)
	if errors.Is(err, taskstore.ErrConflict) {
		shared.Error(w, 409, "IDEMPOTENCY_CONFLICT")
		return
	}
	if err == nil {
		shared.JSON(w, 200, task)
		return
	}
	if err != sql.ErrNoRows {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	f, err := m.root.OpenFile(input.Path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		shared.Error(w, 404, "INSTRUCTION_FILE_UNAVAILABLE")
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 64<<10 {
		shared.Error(w, 400, "INVALID_INSTRUCTION_FILE")
		return
	}
	b, err := io.ReadAll(io.LimitReader(f, (64<<10)+1))
	if err != nil || len(b) == 0 || len(b) > 64<<10 || !utf8.Valid(b) {
		shared.Error(w, 400, "INVALID_INSTRUCTION_FILE")
		return
	}
	task, status, err := m.submitLocked(submit{Instruction: string(b), Key: input.Key, Provider: input.Provider, SourcePath: input.Path})
	if err != nil {
		shared.Error(w, status, err.Error())
		return
	}
	shared.JSON(w, status, task)
}
