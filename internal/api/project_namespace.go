package api

import (
	"agent-workbench/internal/shared"
	"context"
	"net/http"
)

type projectNamespaceRuntime interface {
	EnsureProjectNamespace(context.Context, string) (string, error)
}

// Project metadata is committed first, preserving the creation key across an
// unavailable cluster or partially provisioned namespace. Retrying completes it.
func (s *Server) provisionProjectNamespace(w http.ResponseWriter, r *http.Request, project *shared.Project) bool {
	if !s.cfg.ProvisionProjectNamespaces {
		return true
	}
	provider, ok := s.Runtime.(projectNamespaceRuntime)
	expected := shared.ProjectNamespace(project.ID)
	if !ok || expected == "" {
		shared.Error(w, 503, "PROJECT_NAMESPACE_UNAVAILABLE")
		return false
	}
	namespace, err := provider.EnsureProjectNamespace(r.Context(), project.ID)
	if err != nil || namespace != expected {
		shared.Error(w, 503, "PROJECT_NAMESPACE_UNAVAILABLE")
		return false
	}
	project.DeploymentNamespace, project.NamespaceReady = namespace, true
	if err := s.Store.SaveProjectNamespace(r.Context(), *project); err != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return false
	}
	return true
}

func (s *Server) registerProjectNamespaces(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/projects/{id}/namespace", func(w http.ResponseWriter, r *http.Request) {
		if !s.admin(w, r) {
			return
		}
		s.createMu.Lock()
		defer s.createMu.Unlock()
		id := r.PathValue("id")
		if !s.AuthorizeProject(w, r, id) {
			return
		}
		project, err := s.Store.Project(r.Context(), id)
		if err != nil {
			shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		if !s.cfg.ProvisionProjectNamespaces || shared.ProjectNamespace(id) == "" {
			shared.Error(w, 409, "PROJECT_NAMESPACE_UNAVAILABLE")
			return
		}
		if !s.provisionProjectNamespace(w, r, &project) {
			return
		}
		shared.JSON(w, 200, project)
	})
}
