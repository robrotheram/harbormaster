package api

import (
	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/shared"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

func (s *Server) registerWork(mux *http.ServeMux) {
	repo := orchestration.Repository{DB: s.Store.DB}
	s.registerWorkExecution(mux, repo)
	s.registerWorkConversation(mux, repo)
	s.registerWorkResponsibility(mux, repo)
	mux.HandleFunc("GET /v1/work-items", func(w http.ResponseWriter, r *http.Request) {
		project, ok := s.ProjectForRequest(w, r, r.URL.Query().Get("project_id"))
		if !ok {
			return
		}
		if project == "" {
			shared.Error(w, 400, "PROJECT_REQUIRED")
			return
		}
		items, err := repo.WorkItems(r.Context(), project)
		if err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, map[string]any{"items": items})
	})
	mux.HandleFunc("GET /v1/work-items/{id}", func(w http.ResponseWriter, r *http.Request) {
		project, ok := s.ProjectForRequest(w, r, r.URL.Query().Get("project_id"))
		if !ok {
			return
		}
		if project == "" || !shared.ValidID(r.PathValue("id")) {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		item, err := repo.Work(r.Context(), project, r.PathValue("id"))
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			} else {
				shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			}
			return
		}
		shared.JSON(w, 200, item)
	})
	mux.HandleFunc("POST /v1/work-items/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			ProjectID string `json:"project_id"`
		}
		if !shared.Decode(w, r, &input, 1024) || !s.AuthorizeProject(w, r, input.ProjectID) {
			return
		}
		if !shared.ValidID(r.PathValue("id")) {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		unlock := s.operations.Lock("work:" + r.PathValue("id"))
		defer unlock()
		if !s.authorizeWorkController(w, r, input.ProjectID, r.PathValue("id"), repo) {
			return
		}
		item, err := repo.CancelWork(r.Context(), input.ProjectID, r.PathValue("id"), s.now())
		if errors.Is(err, orchestration.ErrConflict) {
			shared.Error(w, 409, "WORK_STATE_CONFLICT")
			return
		}
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			} else {
				shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			}
			return
		}
		shared.JSON(w, 200, item)
	})

	mux.HandleFunc("POST /v1/work-items", func(w http.ResponseWriter, r *http.Request) {
		var input workItemCreate
		if !shared.Decode(w, r, &input, 400000) {
			return
		}
		actor, _ := r.Context().Value(principalKey{}).(principal)
		item, created, err := s.createWorkItem(r.Context(), actor, input)
		if err != nil {
			writeWorkCreateError(w, err)
			return
		}
		shared.JSON(w, createStatus(created), item)
	})
}

type workItemCreate struct {
	Provider      string   `json:"provider,omitempty"`
	Model         string   `json:"model,omitempty"`
	Instruction   *string  `json:"instruction,omitempty"`
	Presentation  string   `json:"presentation,omitempty"`
	ArtifactPaths []string `json:"artifact_paths"`
	ProjectID     string   `json:"project_id"`
	WorkspaceID   string   `json:"workspace_id"`
	Path          string   `json:"specification_path"`
	Digest        string   `json:"specification_digest"`
	Kind          string   `json:"kind"`
	Key           string   `json:"idempotency_key"`
}

func (s *Server) createWorkItem(ctx context.Context, actor principal, input workItemCreate) (shared.WorkItem, bool, error) {
	if err := s.authorizeProject(ctx, actor, input.ProjectID); err != nil {
		return shared.WorkItem{}, false, err
	}
	repo := orchestration.Repository{DB: s.Store.DB}
	if !shared.ValidWorkSelection(input.Provider, input.Model) || !shared.ValidKey(input.Key) || !shared.ValidID(input.ProjectID) || (input.Presentation != "" && input.Presentation != "interactive-v1") {
		return shared.WorkItem{}, false, requestError{400, "INVALID_REQUEST"}
	}
	unlock := s.operations.Lock("work-create:" + input.ProjectID + ":" + input.Key)
	defer unlock()
	if input.Instruction != nil && (len(*input.Instruction) == 0 || len(*input.Instruction) > 65536 || input.Path != "" || input.Digest != "") {
		return shared.WorkItem{}, false, requestError{400, "INVALID_REQUEST"}
	}
	// Validate operational references before allocating any private resources.
	if input.Instruction == nil && (!shared.ValidProjectFile(input.Path) || len(input.Digest) != 64 || input.Digest != strings.ToLower(input.Digest)) {
		return shared.WorkItem{}, false, requestError{400, "INVALID_REQUEST"}
	}
	if input.Instruction == nil {
		if _, err := hex.DecodeString(input.Digest); err != nil {
			return shared.WorkItem{}, false, requestError{400, "INVALID_REQUEST"}
		}
	}
	if input.Kind != "software" && input.Kind != "research" && input.Kind != "digest" || len(input.ArtifactPaths) > 32 {
		return shared.WorkItem{}, false, requestError{400, "INVALID_REQUEST"}
	}
	for _, p := range input.ArtifactPaths {
		if !shared.ValidProjectFile(p) {
			return shared.WorkItem{}, false, requestError{400, "INVALID_REQUEST"}
		}
	}
	if input.WorkspaceID == "" && s.cfg.SharedWorkspaceID != "" {
		input.WorkspaceID = s.cfg.SharedWorkspaceID
	}
	if input.WorkspaceID != "" {
		ws, err := s.Store.Workspace(ctx, input.WorkspaceID)
		if err != nil || !s.workspaceInProject(ws, input.ProjectID) {
			return shared.WorkItem{}, false, requestError{404, "NOT_FOUND_OR_FORBIDDEN"}
		}
	}
	if len(input.ArtifactPaths) == 0 {
		input.ArtifactPaths = nil
	}
	fingerprintBody, _ := json.Marshal(input)
	fingerprint := sha256.Sum256(fingerprintBody)
	if err := repo.ClaimWorkRequest(ctx, input.ProjectID, input.Key, hex.EncodeToString(fingerprint[:])); err != nil {
		if errors.Is(err, orchestration.ErrConflict) {
			return shared.WorkItem{}, false, requestError{409, "IDEMPOTENCY_CONFLICT"}
		}
		return shared.WorkItem{}, false, requestError{503, "STORAGE_UNAVAILABLE"}
	}

	if input.WorkspaceID == "" {
		hash := sha256.Sum256([]byte(input.ProjectID + "\x00" + input.Key))
		allocated, _, err := s.createWorkspace(ctx, actor, workspaceCreate{Project: input.ProjectID, Key: "work-" + hex.EncodeToString(hash[:]), AgentKind: "codex", TemplateID: "development-v1"})
		if err != nil {
			return shared.WorkItem{}, false, workWorkspaceCreateError{err}
		}
		input.WorkspaceID = allocated.ID
	}
	ws, err := s.Store.Workspace(ctx, input.WorkspaceID)
	if err != nil || !s.workspaceInProject(ws, input.ProjectID) {
		return shared.WorkItem{}, false, requestError{404, "NOT_FOUND_OR_FORBIDDEN"}
	}
	if input.Instruction != nil {
		hash := sha256.Sum256([]byte(input.ProjectID + "\x00" + input.Key))
		input.Path = "work-requests/" + hex.EncodeToString(hash[:]) + ".md"
		instruction := *input.Instruction
		if input.Presentation == "interactive-v1" {
			instruction = "Workbench presentation: interactive-v1.\n" + visualizationGuidance + "\n\nCurrent request:\n" + instruction
		}
		if len(instruction) > 65536 {
			return shared.WorkItem{}, false, requestError{400, "INVALID_REQUEST"}
		}
		digest := sha256.Sum256([]byte(instruction))
		input.Digest = hex.EncodeToString(digest[:])
		release := s.operations.Lock(ws.ID)
		defer release()
		if err := s.start(ctx, &ws); err != nil {
			return shared.WorkItem{}, false, runtimeRequestError(err)
		}
		body, _ := json.Marshal(shared.FileHandoff{Path: input.Path, Content: []byte(instruction)})
		status, _, err := s.Runtime.Agent(ctx, ws.ID, "POST", "/v1/files/write-new", body)
		if err != nil {
			return shared.WorkItem{}, false, requestError{503, "WORKSPACE_UNAVAILABLE"}
		}
		if status != 201 {
			if status != 409 {
				return shared.WorkItem{}, false, requestError{503, "PRIVATE_PUBLICATION_FAILED"}
			}
			body, _ = json.Marshal(shared.FileHandoff{Path: input.Path})
			status, data, err := s.Runtime.Agent(ctx, ws.ID, "POST", "/v1/files/read", body)
			var prior shared.FileHandoff
			if err != nil || status != 200 || json.Unmarshal(data, &prior) != nil {
				return shared.WorkItem{}, false, requestError{503, "PRIVATE_PUBLICATION_UNCONFIRMED"}
			}
			priorDigest := sha256.Sum256(prior.Content)
			if hex.EncodeToString(priorDigest[:]) != input.Digest {
				return shared.WorkItem{}, false, requestError{409, "IDEMPOTENCY_CONFLICT"}
			}
		}
	}
	item, created, err := repo.CreateWork(ctx, shared.WorkItem{ControllerID: actor.human.ID, Provider: input.Provider, Model: input.Model, ArtifactPaths: input.ArtifactPaths, ProjectID: input.ProjectID, WorkspaceID: input.WorkspaceID, SpecificationPath: input.Path, SpecificationDigest: input.Digest, Kind: input.Kind, CreatedAt: s.now()}, input.Key)
	if errors.Is(err, orchestration.ErrConflict) {
		return shared.WorkItem{}, false, requestError{409, "IDEMPOTENCY_CONFLICT"}
	}
	if err != nil {
		if errors.Is(err, orchestration.ErrInvalidWork) {
			return shared.WorkItem{}, false, requestError{400, "INVALID_REQUEST"}
		}
		return shared.WorkItem{}, false, requestError{503, "STORAGE_UNAVAILABLE"}
	}
	return item, created, nil
}

type workWorkspaceCreateError struct{ error }

func (e workWorkspaceCreateError) Unwrap() error { return e.error }

func writeWorkCreateError(w http.ResponseWriter, err error) {
	if _, ok := err.(workWorkspaceCreateError); ok {
		writeRequestErrorBody(w, err)
		return
	}
	writeRequestError(w, err)
}
