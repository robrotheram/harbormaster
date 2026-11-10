package workspace

import (
	"agent-workbench/internal/coordination"
	"agent-workbench/internal/shared"
	"errors"
	"net/http"
	"os"
)

func coordinationError(w http.ResponseWriter, err error) {
	status, code := 503, "COORDINATION_UNAVAILABLE"
	switch {
	case errors.Is(err, coordination.ErrConflict):
		status, code = 409, "PUBLICATION_CONFLICT"
	case errors.Is(err, coordination.ErrInvalid):
		status, code = 400, "INVALID_REQUEST"
	case errors.Is(err, coordination.ErrDenied), errors.Is(err, os.ErrNotExist):
		status, code = 404, "NOT_FOUND_OR_FORBIDDEN"
	}
	shared.Error(w, status, code)
}
func (m *Manager) registerCoordination(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/coordination/blobs/{key}", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Body []byte `json:"body"`
		}
		if !shared.Decode(w, r, &in, 100000) {
			return
		}
		if len(in.Body) == 0 || len(in.Body) > 65536 {
			shared.Error(w, 400, "INVALID_REQUEST")
			return
		}
		root, err := m.coordinationRoot("exports")
		if err != nil {
			coordinationError(w, err)
			return
		}
		defer root.Close()
		if err = (coordination.Files{Root: root}).Put(r.Context(), r.PathValue("key"), in.Body); err != nil {
			coordinationError(w, err)
			return
		}
		shared.JSON(w, 200, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /v1/coordination/blobs/{key}", func(w http.ResponseWriter, r *http.Request) {
		root, err := m.coordinationRoot("exports")
		if err != nil {
			coordinationError(w, err)
			return
		}
		defer root.Close()
		body, err := (coordination.Files{Root: root}).Get(r.Context(), r.PathValue("key"))
		if err != nil {
			coordinationError(w, err)
			return
		}
		shared.JSON(w, 200, map[string][]byte{"body": body})
	})
	mux.HandleFunc("POST /v1/coordination/notes", func(w http.ResponseWriter, r *http.Request) {
		var in coordination.NoteWrite
		if !shared.Decode(w, r, &in, 100000) {
			return
		}
		root, err := m.coordinationRoot("notes")
		if err != nil {
			coordinationError(w, err)
			return
		}
		defer root.Close()
		receipt, err := (coordination.Notes{Root: root}).Publish(in)
		if err != nil {
			coordinationError(w, err)
			return
		}
		shared.JSON(w, 200, receipt)
	})
}
func (m *Manager) coordinationRoot(kind string) (*os.Root, error) {
	if err := m.root.MkdirAll("coordination/"+kind, 0700); err != nil {
		return nil, err
	}
	return m.root.OpenRoot("coordination/" + kind)
}
