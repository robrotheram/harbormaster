package api

import (
	"agent-workbench/internal/forge"
	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/shared"
	"agent-workbench/internal/store"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
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
		return forge.Client{}, "", errors.New("STORAGE_UNAVAILABLE")
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
			return forge.Client{}, "", errors.New("FORGE_HOST_REQUIRED")
		}
	}
	return forge.Client{}, "", errors.New("FORGE_CONNECTION_REMOVED")
}
func (s *Server) registerBoard(mux *http.ServeMux) {
	repo := orchestration.Repository{DB: s.Store.DB}
	mux.HandleFunc("POST /v1/board", func(w http.ResponseWriter, r *http.Request) {
		var in shared.BoardCreate
		if !shared.Decode(w, r, &in, 100000) || !s.AuthorizeProject(w, r, in.ProjectID) {
			return
		}
		in.Title = strings.TrimSpace(in.Title)
		if !shared.ValidID(in.ProjectID) || !shared.ValidKey(in.IdempotencyKey) || in.Title == "" || len(in.Title) > 256 || strings.ContainsAny(in.Title, "\r\n\x00") || !utf8.ValidString(in.Title+in.Description) || len(in.Description) > 60000 {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		unlock := s.operations.Lock("board-create:" + in.ProjectID + ":" + in.IdempotencyKey)
		defer unlock()
		payload, _ := json.Marshal(map[string]any{"project_id": in.ProjectID, "kind": in.Kind, "instruction": "# " + in.Title + "\n\n" + in.Description, "idempotency_key": in.IdempotencyKey})
		req := r.Clone(r.Context())
		u := *r.URL
		u.Path = "/v1/work-items"
		u.RawQuery = ""
		req.URL = &u
		req.Body = io.NopCloser(bytes.NewReader(payload))
		req.ContentLength = int64(len(payload))
		response := &coordinationResponse{headers: make(http.Header)}
		mux.ServeHTTP(response, req)
		if response.Code >= 300 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(response.Code)
			_, _ = w.Write(response.Body.Bytes())
			return
		}
		var work shared.WorkItem
		if json.Unmarshal(response.Body.Bytes(), &work) != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		release := s.operations.Lock("board:" + work.ID)
		defer release()
		b, err := s.Store.BoardItem(r.Context(), in.ProjectID, work.ID)
		if err != nil {
			workError(w, err)
			return
		}
		if b.Revision == 0 {
			b = b.Effective(work)
			b.Title = in.Title
			b.Description = &in.Description
			if err = s.Store.SaveBoardItem(r.Context(), b, 0); err != nil {
				workError(w, err)
				return
			}
			b.Revision = 1
		}
		shared.JSON(w, response.Code, newBoardCard(b, work))
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
			DiscardLocal bool    `json:"discard_local"`
			ProjectID    string  `json:"project_id"`
			Revision     int     `json:"revision"`
			Stage        string  `json:"stage"`
			Title        *string `json:"title"`
			Description  *string `json:"description"`
			Direction    string  `json:"direction"`
		}
		if !shared.Decode(w, r, &in, 100000) || !s.AuthorizeProject(w, r, in.ProjectID) {
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
