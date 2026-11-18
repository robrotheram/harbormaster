package api

import (
	"agent-workbench/internal/forge"
	"agent-workbench/internal/shared"
	"agent-workbench/internal/store"
	"context"
	"errors"
	"net/http"
)

func (s *Server) forgeAccountSummaries(ctx context.Context) ([]store.ForgeAccount, error) {
	external, err := forge.Hosts()
	if err != nil {
		return nil, err
	}
	accounts, err := s.Store.ForgeAccounts(ctx)
	if err != nil {
		return nil, shared.CodedError{Status: http.StatusServiceUnavailable, Code: "STORAGE_UNAVAILABLE"}
	}
	seen := map[string]bool{}
	for _, a := range accounts {
		seen[a.Name] = true
	}
	for _, h := range external {
		if seen[h.Name] {
			return nil, shared.CodedError{Status: http.StatusServiceUnavailable, Code: "FORGE_ACCOUNT_CONFLICT"}
		}
		h.Token = ""
		accounts = append(accounts, store.ForgeAccount{Host: h})
	}
	return accounts, nil
}
func (s *Server) forgeHosts(ctx context.Context) ([]forge.Host, error) {
	hosts, err := forge.Hosts()
	if err != nil {
		return nil, err
	}
	accounts, err := s.Store.ForgeAccounts(ctx)
	if err != nil {
		return nil, shared.CodedError{Status: http.StatusServiceUnavailable, Code: "STORAGE_UNAVAILABLE"}
	}
	for _, a := range accounts {
		sealed, e := s.Store.ForgeAccountCredential(ctx, a.Name)
		if e != nil {
			return nil, shared.CodedError{Status: http.StatusServiceUnavailable, Code: "STORAGE_UNAVAILABLE"}
		}
		a.Token, e = forge.OpenToken(a.Host, s.cfg.Token, sealed)
		if e != nil {
			return nil, e
		}
		hosts = append(hosts, a.Host)
	}
	return hosts, nil
}
func (s *Server) registerForgeAccounts(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/forge-hosts", func(w http.ResponseWriter, r *http.Request) {
		if !s.admin(w, r) {
			return
		}
		var input struct {
			forge.Host
			Revision int `json:"revision"`
		}
		if !decodeRouteBody(w, r, &input) {
			return
		}
		host, err := forge.NormalizeHost(input.Host)
		if err != nil || input.Revision < 0 {
			shared.Error(w, 400, "FORGE_ACCOUNT_INVALID")
			return
		}
		unlock := s.operations.Lock("forge-account:" + host.Name)
		defer unlock()
		summaries, err := s.forgeAccountSummaries(r.Context())
		if err != nil {
			writeForgeError(w, http.StatusServiceUnavailable, err)
			return
		}
		exists := false
		for _, a := range summaries {
			if a.Name == host.Name {
				exists = true
				// Reauthentication never changes the destination of an existing account.
				if !a.Managed || a.Revision != input.Revision || a.Provider != host.Provider || a.APIURL != host.APIURL {
					shared.Error(w, 409, "FORGE_ACCOUNT_CONFLICT")
					return
				}
			}
		}
		if !exists && input.Revision != 0 {
			shared.Error(w, 409, "FORGE_ACCOUNT_CONFLICT")
			return
		}
		if err = (forge.Client{Host: host}).Verify(r.Context()); err != nil {
			writeForgeError(w, http.StatusBadGateway, err)
			return
		}
		sealed, err := forge.SealToken(host, s.cfg.Token)
		if err != nil {
			shared.Error(w, 503, "FORGE_CREDENTIAL_UNAVAILABLE")
			return
		}
		account := store.ForgeAccount{Host: host, Revision: input.Revision + 1, Managed: true}
		account.Token = ""
		if err = s.Store.SaveForgeAccount(r.Context(), account, sealed, input.Revision); err != nil {
			if errors.Is(err, store.ErrForgeAccountConflict) {
				shared.Error(w, 409, "FORGE_ACCOUNT_CONFLICT")
			} else {
				shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			}
			return
		}
		shared.JSON(w, 200, account)
	})
}
