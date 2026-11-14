package mcpserver

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestBoardToolsForwardPlanningChanges(t *testing.T) {
	calls := 0
	session, _ := setup(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" {
			if r.URL.Path != "/v1/board" || r.URL.Query().Get("project_id") != "default" {
				t.Error(r.URL)
			}
			w.Write([]byte(`{"items":[]}`))
			return
		}
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Fatal("invalid body")
		}
		if body["project_id"] != "default" {
			t.Error("missing project")
		}
		if r.URL.Path == "/v1/board/work_one" {
			if _, ok := body["id"]; ok {
				t.Error("ID leaked into strict request body")
			}
			if body["revision"] != float64(3) || body["stage"] != "archived" {
				t.Error(body)
			}
			w.WriteHeader(409)
			w.Write([]byte(`{"code":"BOARD_REVISION_CONFLICT"}`))
			return
		}
		if r.URL.Path != "/v1/board" || body["title"] != "Local issue" || body["idempotency_key"] != "stable" {
			t.Error(body, r.URL)
		}
		w.Write([]byte(`{"id":"work_one","project_id":"default","stage":"planned","revision":1,"work":{"id":"work_one","state":"draft"}}`))
	}))
	if result := call(t, session, "board_list", map[string]any{"project_id": "default"}); result.IsError {
		t.Fatal(result)
	}
	if result := call(t, session, "board_create", map[string]any{"project_id": "default", "title": "Local issue", "description": "Do this later", "kind": "software", "idempotency_key": "stable"}); result.IsError {
		t.Fatal(result)
	}
	if result := call(t, session, "board_update", map[string]any{"project_id": "default", "id": "work_one", "revision": 3, "stage": "archived"}); !result.IsError {
		t.Fatal("conflict hidden", result)
	}
	if calls != 3 {
		t.Fatal(calls)
	}
}
