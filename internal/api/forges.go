package api

import (
	"agent-workbench/internal/forge"
	"agent-workbench/internal/orchestration"
	"agent-workbench/internal/shared"
	"agent-workbench/internal/store"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) registerForges(mux *http.ServeMux) {
	s.registerForgeAccounts(mux)
	mux.HandleFunc("GET /v1/forge-hosts", func(w http.ResponseWriter, r *http.Request) {
		if !s.admin(w, r) {
			return
		}
		hosts, err := s.forgeAccountSummaries(r.Context())
		if err != nil {
			shared.Error(w, 503, err.Error())
			return
		}
		shared.JSON(w, 200, map[string]any{"items": hosts})
	})
	mux.HandleFunc("GET /v1/forge-connections", func(w http.ResponseWriter, r *http.Request) {
		if !s.admin(w, r) {
			return
		}
		items, err := s.Store.ForgeConnections(r.Context())
		if err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		shared.JSON(w, 200, map[string]any{"items": items})
	})
	mux.HandleFunc("POST /v1/forge-connections", func(w http.ResponseWriter, r *http.Request) {
		if !s.admin(w, r) {
			return
		}
		var input store.ForgeConnection
		if !shared.Decode(w, r, &input, 4096) {
			return
		}
		hosts, err := s.forgeHosts(r.Context())
		if err != nil {
			shared.Error(w, 503, err.Error())
			return
		}
		for _, host := range hosts {
			if host.Name == input.Host {
				if !forge.ValidRepository(host.Provider, input.Repository) {
					shared.Error(w, 400, "INVALID_REPOSITORY")
					return
				}
				if _, err = (forge.Client{Host: host}).List(r.Context(), input.Repository, 1); err != nil {
					shared.Error(w, 502, err.Error())
					return
				}
				input.ID = shared.NewID("forge_")
				if s.Store.SaveForge(r.Context(), input) != nil {
					shared.Error(w, 503, "STORAGE_UNAVAILABLE")
					return
				}
				shared.JSON(w, 201, input)
				return
			}
		}
		shared.Error(w, 400, "FORGE_HOST_REQUIRED")
	})
	handle := func(w http.ResponseWriter, r *http.Request) {
		if !s.admin(w, r) {
			return
		}
		var input struct {
			ProjectID string `json:"project_id"`
			Title     string `json:"title"`
			Body      string `json:"body"`
			State     string `json:"state"`
		}
		project := r.URL.Query().Get("project_id")
		if r.Method == "POST" {
			if !shared.Decode(w, r, &input, 70000) {
				return
			}
			project = input.ProjectID
		}
		if strings.HasSuffix(r.URL.Path, "/import") {
			if project == "" {
				shared.Error(w, 400, "PROJECT_REQUIRED")
				return
			}
			if !s.AuthorizeProject(w, r, project) {
				return
			}
		}
		connections, err := s.Store.ForgeConnections(r.Context())
		if err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		var c store.ForgeConnection
		for _, v := range connections {
			if v.ID == r.PathValue("id") {
				c = v
			}
		}
		if c.ID == "" {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		if strings.HasSuffix(r.URL.Path, "/delete") {
			if s.Store.DeleteForge(r.Context(), c.ID) != nil {
				shared.Error(w, 503, "STORAGE_UNAVAILABLE")
				return
			}
			shared.JSON(w, 200, map[string]bool{"deleted": true})
			return
		}
		hosts, err := s.forgeHosts(r.Context())
		if err != nil {
			shared.Error(w, 503, err.Error())
			return
		}
		var host forge.Host
		for _, v := range hosts {
			if v.Name == c.Host {
				host = v
			}
		}
		if host.Name == "" {
			shared.Error(w, 409, "FORGE_HOST_REQUIRED")
			return
		}
		client := forge.Client{Host: host}
		number := 0
		if raw := r.PathValue("number"); raw != "" {
			number, err = strconv.Atoi(raw)
			if err != nil || number < 1 {
				shared.Error(w, 400, "INVALID_REQUEST")
				return
			}
		}
		if r.Method == "GET" {
			page := 1
			if raw := r.URL.Query().Get("page"); raw != "" {
				page, err = strconv.Atoi(raw)
			}
			if err != nil || page < 1 || page > 10000 {
				shared.Error(w, 400, "INVALID_REQUEST")
				return
			}
			items, e := client.List(r.Context(), c.Repository, page)
			if e != nil {
				shared.Error(w, 502, e.Error())
				return
			}
			shared.JSON(w, 200, map[string]any{"items": items, "page": page})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/import") {
			hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", c.ID, number)))
			key := "issue-" + hex.EncodeToString(hash[:])
			workHash := sha256.Sum256([]byte(project + "\x00" + key))
			id := "work_" + hex.EncodeToString(workHash[:16])
			unlock := s.operations.Lock("board:" + id)
			defer unlock()
			repository := orchestration.Repository{DB: s.Store.DB}
			item, lookupErr := repository.Work(r.Context(), project, id)
			if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
				workError(w, lookupErr)
				return
			}
			board, e := s.Store.BoardItem(r.Context(), project, id)
			if e != nil {
				workError(w, e)
				return
			}
			if lookupErr == nil && board.Source != nil {
				shared.JSON(w, 200, item)
				return
			}
			issue, e := client.Get(r.Context(), c.Repository, number)
			if e != nil {
				shared.Error(w, 502, e.Error())
				return
			}
			status := 200
			if errors.Is(lookupErr, sql.ErrNoRows) {
				instruction := fmt.Sprintf("# %s\n\nSource: %s\nRepository: %s\n\n%s", issue.Title, issue.URL, c.Repository, issue.Body)
				actor, _ := r.Context().Value(principalKey{}).(principal)
				var created bool
				item, created, e = s.createWorkItem(r.Context(), actor, workItemCreate{ProjectID: project, Kind: "software", Instruction: &instruction, Key: key})
				if e != nil {
					w.Header().Set("Content-Type", "application/json")
					writeRequestErrorBody(w, e)
					return
				}
				status = createStatus(created)
			}
			board = board.Effective(item)
			board.Title = issue.Title
			board.Description = &issue.Body
			if board.Revision == 0 && issue.State == "closed" {
				board.Stage = "done"
			}
			board.Source = &store.BoardSource{ConnectionID: c.ID, Repository: c.Repository, Number: number, URL: issue.URL, Title: issue.Title, Description: &issue.Body, State: issue.State, SyncedAt: s.now().UTC().Format(time.RFC3339)}
			if e = s.Store.SaveBoardItem(r.Context(), board, board.Revision); e != nil {
				workError(w, e)
				return
			}
			shared.JSON(w, status, item)
			return
		}
		if (number == 0 && strings.TrimSpace(input.Title) == "") || len(input.Title) > 256 || len(input.Body) > 65536 || (number > 0 && input.State != "open" && input.State != "closed") {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		issue, e := client.Save(r.Context(), c.Repository, number, input.Title, input.Body, input.State)
		if e != nil {
			shared.Error(w, 502, e.Error())
			return
		}
		shared.JSON(w, 200, issue)
	}
	mux.HandleFunc("GET /v1/forge-connections/{id}/issues", handle)
	mux.HandleFunc("POST /v1/forge-connections/{id}/issues", handle)
	mux.HandleFunc("POST /v1/forge-connections/{id}/issues/{number}", handle)
	mux.HandleFunc("POST /v1/forge-connections/{id}/issues/{number}/import", handle)
	mux.HandleFunc("POST /v1/forge-connections/{id}/delete", handle)
}
