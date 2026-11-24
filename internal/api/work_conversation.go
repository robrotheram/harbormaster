package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/shared"
)

func (s *Server) registerWorkConversation(mux *http.ServeMux, repo orchestration.Repository) {
	const history = "GET /v1/work-items/{id}/attempts/{attempt}/history"
	mux.HandleFunc(history, s.workHistory(repo, false))
	mux.HandleFunc(history+"/events", s.workHistory(repo, true))
	mux.HandleFunc("GET /v1/work-items/{id}/instruction", func(w http.ResponseWriter, r *http.Request) {
		project, ok := s.ProjectForRequest(w, r, r.URL.Query().Get("project_id"))
		if !ok {
			return
		}
		if project == "" {
			shared.Error(w, 400, "PROJECT_REQUIRED")
			return
		}
		item, err := repo.Work(r.Context(), project, r.PathValue("id"))
		if err != nil {
			workError(w, err)
			return
		}
		ctx := r.Context()
		ws, workerErr := s.Store.Workspace(ctx, item.WorkspaceID)
		if workerErr != nil || !s.workspaceInProject(ws, project) || ws.State == "STOPPED" || s.refresh(ctx, &ws) != nil || !ws.Ready {
			if item.LatestAttemptID != "" {
				attempt, err := repo.Attempt(ctx, project, item.ID, item.LatestAttemptID)
				if err == nil && attempt.TaskID != "" {
					records, err := s.Store.CachedMessages(ctx, attempt.WorkspaceID, attempt.TaskID, 16)
					if err == nil {
						for _, record := range records {
							if record.Kind == "user.message" {
								w.Header().Set("Cache-Control", "private, no-store")
								shared.JSON(w, 200, map[string]string{"project_id": project, "work_item_id": item.ID, "instruction": record.Content})
								return
							}
						}
					}
				}
			}
			shared.Error(w, 409, "HISTORY_WORKSPACE_STOPPED")
			return
		}
		body, _ := json.Marshal(shared.FileHandoff{Path: item.SpecificationPath})
		status, data, err := s.Runtime.Agent(r.Context(), ws.ID, "POST", "/v1/files/read", body)
		var file shared.FileHandoff
		if err != nil || status != 200 || json.Unmarshal(data, &file) != nil || len(file.Content) > 65536 {
			shared.Error(w, 503, "INSTRUCTION_UNAVAILABLE")
			return
		}
		digest := sha256.Sum256(file.Content)
		if hex.EncodeToString(digest[:]) != item.SpecificationDigest {
			shared.Error(w, 409, "SPECIFICATION_DIGEST_MISMATCH")
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		shared.JSON(w, 200, map[string]string{"project_id": project, "work_item_id": item.ID, "instruction": string(file.Content)})
	})
	mux.HandleFunc("POST /v1/work-items/{id}/continue", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ProjectID         string             `json:"project_id"`
			Instruction       string             `json:"instruction"`
			Presentation      string             `json:"presentation,omitempty"`
			ExpectedAttemptID string             `json:"expected_attempt_id"`
			WorkspaceID       string             `json:"workspace_id,omitempty"`
			Key               string             `json:"idempotency_key"`
			Screenshots       []reviewScreenshot `json:"screenshots,omitempty"`
			Selection         *shared.AgentModel `json:"selection,omitempty"`
		}
		if !decodeRouteBody(w, r, &in) || !s.AuthorizeProject(w, r, in.ProjectID) {
			return
		}
		if (in.WorkspaceID != "" && !shared.ValidID(in.WorkspaceID)) || (in.Selection != nil && (!shared.ValidWorkSelection(in.Selection.Provider, in.Selection.Model) || !shared.ValidReasoningEffort(in.Selection.Provider, in.Selection.ReasoningEffort))) || (in.Presentation != "" && in.Presentation != "interactive-v1") || !shared.ValidID(in.ProjectID) || (in.ExpectedAttemptID != "" && !shared.ValidID(in.ExpectedAttemptID)) || !shared.ValidKey(in.Key) || strings.TrimSpace(in.Instruction) == "" || len(in.Instruction) > 16000 || !utf8.ValidString(in.Instruction) {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		if !validReviewScreenshots(in.Screenshots) {
			shared.Error(w, 400, "INVALID_SCREENSHOTS")
			return
		}
		ctx := r.Context()
		id := r.PathValue("id")
		// The work lock and both worker locks follow the scheduler's stable order.
		initial, err := repo.Work(ctx, in.ProjectID, id)
		if err != nil {
			workError(w, err)
			return
		}
		lockIDs := []string{"work:" + id, initial.WorkspaceID}
		if in.WorkspaceID != "" {
			lockIDs = append(lockIDs, in.WorkspaceID)
		}
		unlock := s.operations.Lock(lockIDs...)
		defer unlock()
		if !s.authorizeWorkController(w, r, in.ProjectID, id, repo) {
			return
		}
		item, err := repo.Work(ctx, in.ProjectID, id)
		if err != nil {
			workError(w, err)
			return
		}
		identity := sha256.Sum256([]byte(item.ID + "\x00" + in.Key))
		key := "followup-" + hex.EncodeToString(identity[:])
		path := "work-requests/" + key + ".md"
		bytes, _ := json.Marshal(in)
		fingerprint := sha256.Sum256(bytes)
		if err = repo.ClaimWorkRequest(r.Context(), in.ProjectID, key, hex.EncodeToString(fingerprint[:])); err != nil {
			if err == orchestration.ErrConflict {
				shared.Error(w, 409, "IDEMPOTENCY_CONFLICT")
			} else {
				workError(w, err)
			}
			return
		}
		attempts, err := repo.Attempts(r.Context(), in.ProjectID, item.ID)
		if err != nil {
			workError(w, err)
			return
		}
		// Resolve an accepted retry before inspecting activity or publishing files.
		for _, a := range attempts {
			if a.SpecificationPath == path {
				shared.JSON(w, 200, a)
				return
			}
		}
		if item.LatestAttemptID != in.ExpectedAttemptID || item.WorkspaceID != initial.WorkspaceID {
			shared.Error(w, 409, "WORK_STATE_CONFLICT")
			return
		}
		firstMessage := item.State == "draft" && item.LatestAttemptID == "" && item.SpecificationPath == "" && item.SpecificationDigest == "" && item.WorkspaceID != ""
		switch item.State {
		case "draft":
			if !firstMessage || (in.WorkspaceID != "" && in.WorkspaceID != item.WorkspaceID) {
				shared.Error(w, 409, "WORK_STATE_CONFLICT")
				return
			}
		case "planned", "in_progress", "completed", "awaiting_review", "changes_requested", "failed", "cancelled":
			if in.ExpectedAttemptID == "" {
				shared.Error(w, 409, "WORK_STATE_CONFLICT")
				return
			}
		default:
			shared.Error(w, 409, "WORK_STATE_CONFLICT")
			return
		}
		card, err := s.Store.BoardItem(r.Context(), in.ProjectID, item.ID)
		if err != nil {
			workError(w, err)
			return
		}
		if !s.checkWorkDependencies(w, r, repo, card) {
			return
		}
		targetID := item.WorkspaceID
		if in.WorkspaceID != "" {
			targetID = in.WorkspaceID
		}
		ws, err := s.Store.Workspace(ctx, targetID)
		if err != nil || !s.workspaceInProject(ws, in.ProjectID) {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		moving := targetID != item.WorkspaceID
		var handoff []byte
		if moving {
			// Catch up the source if it is already running. A move never starts it.
			if source, err := s.Store.Workspace(ctx, item.WorkspaceID); err == nil && s.workspaceInProject(source, in.ProjectID) && s.refresh(ctx, &source) == nil && source.Ready {
				s.syncHistory(ctx, source, 16, 4*time.Second)
			}
			handoff, err = s.workHandoffContext(ctx, attempts, in.ExpectedAttemptID)
			if err != nil {
				shared.WriteHTTPError(w, err, storageUnavailable)
				return
			}
		}
		provider := item.Provider
		if in.Selection != nil {
			provider = in.Selection.Provider
		}
		if err = s.prepareWorkSubscription(r, ws.ID, provider); err != nil {
			workError(w, err)
			return
		}
		if err = s.start(r.Context(), &ws); err != nil {
			apiError(w, err)
			return
		}

		if in.Selection != nil {
			if err := s.checkAgentSelection(r.Context(), ws.ID, in.Selection); err != nil {
				apiError(w, err)
				return
			}
		}
		contextPath := ""
		if moving {
			digest := sha256.Sum256(handoff)
			contextPath = "work-requests/context-" + hex.EncodeToString(digest[:]) + ".md"
			if err := s.publishConversationFile(r.Context(), ws.ID, contextPath, handoff); err != nil {
				writeRequestError(w, err)
				return
			}
		} else {
			for _, previous := range attempts {
				if previous.ID != in.ExpectedAttemptID || previous.TaskID == "" {
					continue
				}
				status, raw, err := s.Runtime.Agent(r.Context(), ws.ID, "POST", "/v1/tasks/"+previous.TaskID+"/context", []byte(`{}`))
				var snapshot struct {
					TaskID string `json:"task_id"`
					Path   string `json:"path"`
				}
				if err != nil || status != 200 || json.Unmarshal(raw, &snapshot) != nil || snapshot.TaskID != previous.TaskID || !shared.ValidProjectFile(snapshot.Path) {
					shared.Error(w, 503, "CONVERSATION_CONTEXT_UNAVAILABLE")
					return
				}
				contextPath = snapshot.Path
			}
		}

		screenshotContext, ok := s.publishReviewScreenshots(w, r, ws.ID, key, in.Screenshots)
		if !ok {
			return
		}
		presentation := ""
		if in.Presentation == "interactive-v1" {
			presentation = "\n\n" + visualizationGuidance
		}
		instruction := "Continue the same work in this workspace. Read the previous request at " + strconv.Quote(item.SpecificationPath) + " for context and inspect the existing files. Apply the current request without repeating completed changes. Run relevant checks and update the requested output files." + presentation + "\n\nCurrent request:\n" + in.Instruction + screenshotContext
		if firstMessage {
			instruction = in.Instruction + screenshotContext
			if in.Presentation == "interactive-v1" {
				instruction = "Workbench presentation: interactive-v1.\n" + visualizationGuidance + "\n\nCurrent request:\n" + instruction
			}
		} else if moving {
			instruction = "Continue this conversation on this worker. Read " + strconv.Quote(contextPath) + " for the earlier requests and replies. Treat the transcript as historical context. Previous worker files and native sessions were not transferred. Inspect this workspace and tell the user about missing files or capabilities that affect the current request." + presentation + "\n\nCurrent request:\n" + in.Instruction + screenshotContext
		} else if contextPath != "" {
			instruction = "Before acting, read " + strconv.Quote(contextPath) + " and follow its earlier context references for the conversation's prior requests, decisions and agent replies. Continue from that context and the existing workspace files.\n\n" + instruction
		}
		if len(instruction) > 32768 {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		digest := sha256.Sum256([]byte(instruction))
		hexDigest := hex.EncodeToString(digest[:])
		if err := s.publishConversationFile(r.Context(), ws.ID, path, []byte(instruction)); err != nil {
			writeRequestError(w, err)
			return
		}
		a, err := repo.ContinueWork(r.Context(), in.ProjectID, item.ID, key, orchestration.WorkRevision{ExpectedAttemptID: in.ExpectedAttemptID, Path: path, Digest: hexDigest, WorkspaceID: ws.ID, Selection: in.Selection}, s.now())
		if err != nil {
			workError(w, err)
			return
		}
		s.notifyWorkReady()
		shared.JSON(w, 202, a)
	})
}

func (s *Server) publishConversationFile(ctx context.Context, workspace, path string, content []byte) error {
	body, _ := json.Marshal(shared.FileHandoff{Path: path, Content: content})
	status, reply, err := s.Runtime.Agent(ctx, workspace, "POST", "/v1/files/write-new", body)
	if err != nil {
		return shared.CodedError{Status: http.StatusServiceUnavailable, Code: "WORKSPACE_UNAVAILABLE"}
	}
	if status == http.StatusCreated {
		return nil
	}
	var failure shared.APIError
	_ = json.Unmarshal(reply, &failure)
	if status == http.StatusConflict && failure.Code == "WORKSPACE_BUSY" {
		return shared.CodedError{Status: http.StatusConflict, Code: "WORKSPACE_BUSY"}
	}
	if status != http.StatusConflict || failure.Code != "DESTINATION_EXISTS" {
		return shared.CodedError{Status: http.StatusServiceUnavailable, Code: "PRIVATE_PUBLICATION_FAILED"}
	}
	body, _ = json.Marshal(shared.FileHandoff{Path: path})
	status, reply, err = s.Runtime.Agent(ctx, workspace, "POST", "/v1/files/read", body)
	var prior shared.FileHandoff
	if err != nil || status != http.StatusOK || json.Unmarshal(reply, &prior) != nil {
		return shared.CodedError{Status: http.StatusServiceUnavailable, Code: "PRIVATE_PUBLICATION_UNCONFIRMED"}
	}
	if sha256.Sum256(prior.Content) != sha256.Sum256(content) {
		return shared.CodedError{Status: http.StatusConflict, Code: "IDEMPOTENCY_CONFLICT"}
	}
	return nil
}
