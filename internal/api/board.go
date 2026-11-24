package api

import (
	"agent-workbench/internal/forge"
	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/shared"
	"agent-workbench/internal/store"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type boardCard struct {
	store.BoardItem
	Work      shared.WorkItem `json:"work"`
	Dirty     bool            `json:"dirty"`
	Attention bool            `json:"attention"`
}

func newBoardCard(b store.BoardItem, work shared.WorkItem) boardCard {
	b = b.Effective(work)
	attention := work.State == "failed" || work.State == "blocked" || work.State == "cancelled" || work.State == "unknown"
	return boardCard{BoardItem: b, Work: work, Dirty: b.Dirty(), Attention: attention}
}
func (s *Server) boardForge(ctx context.Context, id string) (forge.Client, string, error) {
	connections, err := s.Store.ForgeConnections(ctx)
	if err != nil {
		return forge.Client{}, "", shared.CodedError{Status: http.StatusBadGateway, Code: "STORAGE_UNAVAILABLE"}
	}
	for _, c := range connections {
		if c.ID == id {
			hosts, err := s.forgeHosts(ctx)
			if err != nil {
				return forge.Client{}, "", err
			}
			for _, h := range hosts {
				if h.Name == c.Host {
					return forge.Client{Host: h}, c.Repository, nil
				}
			}
			return forge.Client{}, "", shared.CodedError{Status: http.StatusBadGateway, Code: "FORGE_HOST_REQUIRED"}
		}
	}
	return forge.Client{}, "", shared.CodedError{Status: http.StatusBadGateway, Code: "FORGE_CONNECTION_REMOVED"}
}
func (s *Server) registerBoard(mux *http.ServeMux) {
	s.registerBoardStart(mux)
	repo := orchestration.Repository{DB: s.Store.DB}
	mux.HandleFunc("POST /v1/board", func(w http.ResponseWriter, r *http.Request) {
		var in shared.BoardCreate
		if !decodeRouteBody(w, r, &in) || !s.AuthorizeProject(w, r, in.ProjectID) {
			return
		}
		in.Title = strings.TrimSpace(in.Title)
		if !shared.ValidID(in.ProjectID) || !shared.ValidKey(in.IdempotencyKey) || in.Title == "" || len(in.Title) > 256 || strings.ContainsAny(in.Title, "\r\n\x00") || !utf8.ValidString(in.Title+in.Description) || len(in.Description) > 60000 {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}

		if (in.Kind != "software" && in.Kind != "research" && in.Kind != "digest" && in.Kind != shared.WorkKindDataScience) || strings.ContainsRune(in.Description, 0) {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		unlock := s.operations.Lock("board-create:" + in.ProjectID + ":" + in.IdempotencyKey)
		defer unlock()
		fingerprint, _ := json.Marshal(in)
		hash := sha256.Sum256(fingerprint)
		if err := repo.ClaimWorkRequest(r.Context(), in.ProjectID, in.IdempotencyKey, hex.EncodeToString(hash[:])); err != nil {
			if errors.Is(err, orchestration.ErrConflict) {
				shared.Error(w, 409, "IDEMPOTENCY_CONFLICT")
			} else {
				workError(w, err)
			}
			return
		}
		p, _ := r.Context().Value(principalKey{}).(principal)
		work, created, err := s.Store.CreatePlannedTask(r.Context(), in, p.human.ID, nil, s.now())
		if err != nil {
			workError(w, err)
			return
		}
		b, err := s.Store.BoardItem(r.Context(), in.ProjectID, work.ID)
		if err != nil {
			workError(w, err)
			return
		}
		status := 200
		if created {
			status = 201
		}
		shared.JSON(w, status, newBoardCard(b, work))
	})
	mux.HandleFunc("GET /v1/board", func(w http.ResponseWriter, r *http.Request) {
		project, ok := s.ProjectForRequest(w, r, r.URL.Query().Get("project_id"))
		if !ok {
			return
		}
		if project == "" {
			shared.Error(w, 400, "PROJECT_REQUIRED")
			return
		}
		work, err := repo.WorkItems(r.Context(), project)
		if err != nil {
			workError(w, err)
			return
		}
		cards := []boardCard{}
		for _, item := range work {
			b, err := s.Store.BoardItem(r.Context(), project, item.ID)
			if err != nil {
				workError(w, err)
				return
			}
			cards = append(cards, newBoardCard(b, item))
		}
		shared.JSON(w, 200, map[string]any{"items": cards})
	})
	handle := func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			OwnerID             *string   `json:"owner_id"`
			Owner               *string   `json:"owner"`
			Priority            *string   `json:"priority"`
			RequireDependencies *bool     `json:"require_dependencies"`
			Dependencies        *[]string `json:"dependencies"`
			DiscardLocal        bool      `json:"discard_local"`
			ProjectID           string    `json:"project_id"`
			Revision            int       `json:"revision"`
			Stage               string    `json:"stage"`
			Title               *string   `json:"title"`
			Description         *string   `json:"description"`
			Direction           string    `json:"direction"`
		}
		if !decodeRouteBody(w, r, &in) || !s.AuthorizeProject(w, r, in.ProjectID) {
			return
		}
		if in.ProjectID == "" || !shared.ValidID(r.PathValue("id")) || in.Revision < 0 {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		sync := strings.HasSuffix(r.URL.Path, "/sync")
		if sync && !s.admin(w, r) {
			return
		}
		unlock := s.operations.Lock("board:" + r.PathValue("id"))
		defer unlock()
		work, err := repo.Work(r.Context(), in.ProjectID, r.PathValue("id"))
		if err != nil {
			workError(w, err)
			return
		}
		b, err := s.Store.BoardItem(r.Context(), in.ProjectID, work.ID)
		if err != nil {
			workError(w, err)
			return
		}
		b = b.Effective(work)
		moveWork := !sync && in.Stage != b.Stage && in.Stage != "archived" && ((store.BoardStage(work.State) == "done" && in.Stage != "done") || work.State == "planned" || work.State == "in_progress" || work.State == "awaiting_review")
		if b.Revision != in.Revision {
			shared.Error(w, 409, "BOARD_REVISION_CONFLICT")
			return
		}
		save := func() bool {
			err := s.Store.SaveBoardItem(r.Context(), b, in.Revision)
			if err != nil {
				if errors.Is(err, store.ErrBoardConflict) {
					shared.Error(w, 409, "BOARD_REVISION_CONFLICT")
				} else {
					workError(w, err)
				}
				return false
			}
			b.Revision = in.Revision + 1
			return true
		}
		if !sync && in.Description != nil && (len(*in.Description) > 60000 || !utf8.ValidString(*in.Description) || strings.ContainsRune(*in.Description, 0)) {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		if !sync {
			if in.OwnerID != nil {
				b.OwnerID = *in.OwnerID
				b.Owner = ""
				if b.OwnerID != "" {
					members, err := s.Store.ProjectMembers(r.Context(), in.ProjectID)
					if err != nil {
						workError(w, err)
						return
					}
					for _, member := range members {
						if member.UserID == b.OwnerID && member.Status == "active" {
							b.Owner = member.Email
							break
						}
					}
					if b.Owner == "" {
						shared.Error(w, 400, "ACTIVE_MEMBER_REQUIRED")
						return
					}
				}
			}
			if in.Owner != nil && in.OwnerID == nil {
				owner := strings.TrimSpace(*in.Owner)
				if len(owner) > 160 || !utf8.ValidString(owner) || strings.ContainsAny(owner, "\r\n\x00") {
					shared.Error(w, 400, "INVALID_OWNER")
					return
				}
				b.Owner = owner
				b.OwnerID = ""
			}
			if in.Priority != nil {
				if *in.Priority != "" && *in.Priority != "low" && *in.Priority != "normal" && *in.Priority != "high" {
					shared.Error(w, 400, "INVALID_PRIORITY")
					return
				}
				b.Priority = *in.Priority
			}
			if in.RequireDependencies != nil {
				b.RequireDependencies = *in.RequireDependencies
			}
			if in.Dependencies != nil {
				if len(*in.Dependencies) > 32 {
					shared.Error(w, 400, "INVALID_DEPENDENCIES")
					return
				}
				seen := map[string]bool{}
				for _, id := range *in.Dependencies {
					if id == work.ID || seen[id] || !shared.ValidID(id) {
						shared.Error(w, 400, "INVALID_DEPENDENCIES")
						return
					}
					if _, err := repo.Work(r.Context(), in.ProjectID, id); err != nil {
						shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
						return
					}
					seen[id] = true
				}
				b.Dependencies = *in.Dependencies
			}
			if in.Description != nil {
				b.Description = in.Description
			}
			if !store.ValidBoardStage(in.Stage) {
				shared.Error(w, 400, "INVALID_REQUEST")
				return
			}
			if in.Stage == "archived" && b.Stage != "archived" {
				b.ArchivedFrom = b.Stage
			}
			b.Stage = in.Stage
			if in.Title != nil {
				title := strings.TrimSpace(*in.Title)
				if title == "" || len(title) > 256 || strings.ContainsAny(title, "\r\n\x00") {
					shared.Error(w, 400, "INVALID_REQUEST")
					return
				}
				b.Title = title
			}
		} else {
			if b.Source == nil || (in.Direction != "pull" && in.Direction != "push") {
				shared.Error(w, 400, "INVALID_REQUEST")
				return
			}
			fail := func(code string, status int) {
				b.Source.Error = code
				if save() {
					shared.Error(w, status, code)
				}
			}
			client, repository, e := s.boardForge(r.Context(), b.Source.ConnectionID)
			if e != nil {
				fail(e.Error(), 502)
				return
			}
			remote, e := client.Get(r.Context(), repository, b.Source.Number)
			if e != nil {
				fail(e.Error(), 502)
				return
			}
			changed := remote.Title != b.Source.Title || remote.State != b.Source.State
			descriptionChanged := b.Description != nil && (b.Source.Description == nil || remote.Body != *b.Source.Description)
			descriptionMatches := b.Description == nil || remote.Body == *b.Description
			if in.Direction == "pull" {
				if b.Dirty() && !in.DiscardLocal {
					fail("BOARD_SYNC_LOCAL_CHANGES", 409)
					return
				}
				b.Title = remote.Title
				b.Description = &remote.Body
				if b.Stage == "archived" {
					if remote.State == "closed" {
						b.ArchivedFrom = "done"
					} else if b.ArchivedFrom == "done" {
						b.ArchivedFrom = "planned"
					}
				}
				if b.Stage != "archived" {
					if remote.State == "closed" {
						b.Stage = "done"
					} else if b.Stage == "done" {
						b.Stage = "planned"
					}
				}
			} else {
				// A retry after a lost response is safe when the remote already matches.
				desired := b.RemoteState()
				if (changed && (remote.Title != b.Title || remote.State != desired)) || (descriptionChanged && !descriptionMatches) {
					fail("BOARD_SYNC_CONFLICT", 409)
					return
				}
				if remote.Title != b.Title || remote.State != desired || !descriptionMatches {
					remote, e = client.SaveDescription(r.Context(), repository, b.Source.Number, b.Title, b.Description, desired)
					if e != nil {
						fail(e.Error(), 502)
						return
					}
				}
			}
			b.Source.Description = &remote.Body
			b.Source.Title = remote.Title
			b.Source.State = remote.State
			b.Source.URL = remote.URL
			b.Source.SyncedAt = s.now().UTC().Format(time.RFC3339)
			b.Source.Error = ""
		}
		if moveWork {
			work, err = s.Store.MoveBoardWork(r.Context(), b, in.Revision, work, s.now())
			if err != nil {
				if errors.Is(err, store.ErrBoardConflict) {
					shared.Error(w, 409, "BOARD_REVISION_CONFLICT")
				} else {
					workError(w, err)
				}
				return
			}
			b.Revision = in.Revision + 1
			shared.JSON(w, 200, newBoardCard(b.Effective(work), work))
			return
		}
		// Repository metadata cannot undo a local review outcome either.
		b = b.Effective(work)
		if save() {
			shared.JSON(w, 200, newBoardCard(b, work))
		}
	}
	mux.HandleFunc("POST /v1/board/{id}", handle)
	mux.HandleFunc("POST /v1/board/{id}/sync", handle)
}
