package api

import (
	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/shared"
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
		var input struct {
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
		if !shared.Decode(w, r, &input, 400000) || !s.AuthorizeProject(w, r, input.ProjectID) {
			return
		}
		if !shared.ValidKey(input.Key) || !shared.ValidID(input.ProjectID) || (input.Presentation != "" && input.Presentation != "interactive-v1") {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		unlock := s.operations.Lock("work-create:" + input.ProjectID + ":" + input.Key)
		defer unlock()
		if input.Instruction != nil && (len(*input.Instruction) == 0 || len(*input.Instruction) > 65536 || input.Path != "" || input.Digest != "") {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		// Validate operational references before allocating any private resources.
		if input.Instruction == nil && (!shared.ValidProjectFile(input.Path) || len(input.Digest) != 64 || input.Digest != strings.ToLower(input.Digest)) {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		if input.Instruction == nil {
			if _, err := hex.DecodeString(input.Digest); err != nil {
				shared.Error(w, 400, "INVALID_REQUEST")
				return
			}
		}
		if input.Kind != "software" && input.Kind != "research" && input.Kind != "digest" || len(input.ArtifactPaths) > 32 {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		for _, p := range input.ArtifactPaths {
			if !shared.ValidProjectFile(p) {
				shared.Error(w, 400, "INVALID_REQUEST")
				return
			}
		}
		if input.WorkspaceID != "" {
			ws, err := s.Store.Workspace(r.Context(), input.WorkspaceID)
			if err != nil || ws.ProjectID != input.ProjectID {
				shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
				return
			}
		}
		if len(input.ArtifactPaths) == 0 {
			input.ArtifactPaths = nil
		}
		fingerprintBody, _ := json.Marshal(input)
		fingerprint := sha256.Sum256(fingerprintBody)
		if err := repo.ClaimWorkRequest(r.Context(), input.ProjectID, input.Key, hex.EncodeToString(fingerprint[:])); err != nil {
			if errors.Is(err, orchestration.ErrConflict) {
				shared.Error(w, 409, "IDEMPOTENCY_CONFLICT")
			} else {
				shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			}
			return
		}

		if input.WorkspaceID == "" {
			response := &coordinationResponse{headers: make(http.Header)}
			hash := sha256.Sum256([]byte(input.ProjectID + "\x00" + input.Key))
			s.createProjectWorkspace(response, r, workspaceCreate{Project: input.ProjectID, Key: "work-" + hex.EncodeToString(hash[:]), AgentKind: "codex", TemplateID: "development-v1"})
			if response.Code >= 300 {
				w.WriteHeader(response.Code)
				_, _ = w.Write(response.Body.Bytes())
				return
			}
			var allocated shared.Workspace
			if json.Unmarshal(response.Body.Bytes(), &allocated) != nil {
				shared.Error(w, 503, "STORAGE_UNAVAILABLE")
				return
			}
			input.WorkspaceID = allocated.ID
		}
		ws, err := s.Store.Workspace(r.Context(), input.WorkspaceID)
		if err != nil || ws.ProjectID != input.ProjectID {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		if input.Instruction != nil {
			hash := sha256.Sum256([]byte(input.ProjectID + "\x00" + input.Key))
			input.Path = "work-requests/" + hex.EncodeToString(hash[:]) + ".md"
			instruction := *input.Instruction
			if input.Presentation == "interactive-v1" {
				instruction = "Workbench presentation: interactive-v1.\n" + visualizationGuidance + "\n\nCurrent request:\n" + instruction
			}
			if len(instruction) > 65536 {
				shared.Error(w, 400, "INVALID_REQUEST")
				return
			}
			digest := sha256.Sum256([]byte(instruction))
			input.Digest = hex.EncodeToString(digest[:])
			release := s.operations.Lock(ws.ID)
			defer release()
			if err := s.start(r.Context(), &ws); err != nil {
				apiError(w, err)
				return
			}
			body, _ := json.Marshal(shared.FileHandoff{Path: input.Path, Content: []byte(instruction)})
			status, _, err := s.Runtime.Agent(r.Context(), ws.ID, "POST", "/v1/files/write-new", body)
			if err != nil {
				shared.Error(w, 503, "WORKSPACE_UNAVAILABLE")
				return
			}
			if status != 201 {
				if status != 409 {
					shared.Error(w, 503, "PRIVATE_PUBLICATION_FAILED")
					return
				}
				body, _ = json.Marshal(shared.FileHandoff{Path: input.Path})
				status, data, err := s.Runtime.Agent(r.Context(), ws.ID, "POST", "/v1/files/read", body)
				var prior shared.FileHandoff
				if err != nil || status != 200 || json.Unmarshal(data, &prior) != nil {
					shared.Error(w, 503, "PRIVATE_PUBLICATION_UNCONFIRMED")
					return
				}
				priorDigest := sha256.Sum256(prior.Content)
				if hex.EncodeToString(priorDigest[:]) != input.Digest {
					shared.Error(w, 409, "IDEMPOTENCY_CONFLICT")
					return
				}
			}
		}
		item, created, err := repo.CreateWork(r.Context(), shared.WorkItem{ArtifactPaths: input.ArtifactPaths, ProjectID: input.ProjectID, WorkspaceID: input.WorkspaceID, SpecificationPath: input.Path, SpecificationDigest: input.Digest, Kind: input.Kind, CreatedAt: s.now()}, input.Key)
		if errors.Is(err, orchestration.ErrConflict) {
			shared.Error(w, 409, "IDEMPOTENCY_CONFLICT")
			return
		}
		if err != nil {
			if errors.Is(err, orchestration.ErrInvalidWork) {
				shared.Error(w, 400, "INVALID_REQUEST")
			} else {
				shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			}
			return
		}
		status := 200
		if created {
			status = 201
		}
		shared.JSON(w, status, item)
	})
}
