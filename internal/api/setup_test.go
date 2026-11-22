package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestWorkerSetupUsesProjectAdminAccessAndDoesNotStartAWorker(t *testing.T) {
	s, rt := setup(t)
	ws := create(t, s, "setup-security")
	path := "/v1/workspaces/" + ws.ID + "/setup"
	s.cfg.ProjectTokens = map[string]string{"project-credential": ws.ProjectID}
	body := `{"operation":"tools-restore"}`
	if response := domainRequest(t, s, "project-credential", http.MethodPost, path, body); response.Code != 403 ||
		!strings.Contains(response.Body.String(), "PROJECT_ADMIN_REQUIRED") {
		t.Fatal(response.Code, response.Body.String())
	}
	if response := request(t, s, http.MethodPost, path, body); response.Code != 409 ||
		!strings.Contains(response.Body.String(), "WORKSPACE_NOT_READY") {
		t.Fatal(response.Code, response.Body.String())
	}
	if observation, err := rt.Inspect(t.Context(), ws.ID); err != nil || observation.Ready {
		t.Fatal("setup request started a worker", observation, err)
	}
}
