package workspace

import (
	"agent-workbench/internal/workspace/taskstore"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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
		Digest   string `json:"specification_digest"`
		Path     string `json:"instruction_path"`
		Model    string `json:"model,omitempty"`
		Provider string `json:"provider"`
		Key      string `json:"idempotency_key"`
	}
	if !shared.Decode(w, r, &input, 2048) {
		return
	}
	if (input.Model != "" && !shared.ValidWorkSelection(input.Provider, input.Model)) || !shared.ValidProjectFile(input.Path) || !shared.ValidKey(input.Key) || (input.Provider != "" && input.Provider != "codex" && input.Provider != "claude" && !(input.Provider == "hermes" && m.cfg.AgentKind == "hermes")) {
		shared.Error(w, 400, "INVALID_REQUEST")
		return
	}
	if input.Digest != "" {
		b, e := hex.DecodeString(input.Digest)
		if e != nil || len(b) != 32 || hex.EncodeToString(b) != input.Digest {
			shared.Error(w, 400, "INVALID_SPECIFICATION_DIGEST")
			return
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if input.Provider == "" {
		input.Provider = m.cfg.DefaultProvider
	}
	task, err := m.tasks.FileAcceptance(r.Context(), input.Key, input.Path, input.Provider, input.Digest, input.Model)
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
	// Resolve receipts before checking activity: recovery of this task's lost
	// acknowledgement must succeed while that same task is still executing.
	activity := m.workActivityLocked()
	if !activity.Known || activity.ActiveRuns > 0 || activity.BackgroundJobs > 0 {
		shared.Error(w, 409, "WORKSPACE_BUSY")
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
	sum := sha256.Sum256(b)
	if input.Digest != "" && hex.EncodeToString(sum[:]) != input.Digest {
		shared.Error(w, 409, "SPECIFICATION_DIGEST_MISMATCH")
		return
	}
	task, status, err := m.submitLocked(submit{Instruction: string(b), Key: input.Key, Provider: input.Provider, Model: input.Model, SourcePath: input.Path, SpecificationDigest: input.Digest})
	if err != nil {
		shared.WriteHTTPError(w, err, shared.CodedError{Status: status, Code: err.Error()})
		return
	}
	shared.JSON(w, status, task)
}
