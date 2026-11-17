package api

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestTeamCoordinationMetadataIsScopedAndRevisionChecked(t *testing.T) {
	s, _ := setup(t)
	work := boardWork(t, s)
	path := "/v1/board/" + work.ID
	payload := `{"project_id":"default","revision":0,"stage":"planned","owner":"Maya","priority":"high","dependencies":[]}`
	response := request(t, s, "POST", path, payload)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body)
	}
	stored, err := s.Store.BoardItem(context.Background(), "default", work.ID)
	if err != nil || stored.Owner != "Maya" || stored.Priority != "high" {
		t.Fatal(stored, err)
	}
	if got := request(t, s, "POST", path, payload); got.Code != 409 {
		t.Fatal("stale update accepted", got.Code)
	}
	for _, dependency := range []string{work.ID, "work_missing"} {
		body := fmt.Sprintf(`{"project_id":"default","revision":1,"stage":"planned","dependencies":[%q]}`, dependency)
		got := request(t, s, "POST", path, body)
		if got.Code != 400 && got.Code != 404 {
			t.Fatal("invalid dependency accepted", got.Code)
		}
	}
	foreign := domainProject(t, s, "team-foreign")
	s.cfg.ProjectTokens = map[string]string{"foreign": foreign.ID}
	if got := domainRequest(t, s, "foreign", "POST", path, strings.Replace(payload, `"revision":0`, `"revision":1`, 1)); got.Code != 404 {
		t.Fatal("foreign edit accepted", got.Code)
	}
}
