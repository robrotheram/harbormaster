package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"agent-workbench/internal/shared"
)

func TestEventsFilters(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "metadata.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()

	workspace := shared.Workspace{ID: "ws_events", ProjectID: "default", HomeID: "home_events", AgentKind: "codex"}
	if err := s.Create(ctx, workspace, "create-events-workspace"); err != nil {
		t.Fatal(err)
	}
	event := shared.ActivityEvent{ID: "event_started", At: time.Now(), Source: "codex", Kind: "task.started"}
	if err := s.SaveEvents(ctx, workspace, []shared.ActivityEvent{event}); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name, project, workspace string
		want                     int
	}{
		{name: "unfiltered", want: 1},
		{name: "project only", project: workspace.ProjectID, want: 1},
		{name: "workspace only", workspace: workspace.ID, want: 1},
		{name: "both filters", project: workspace.ProjectID, workspace: workspace.ID, want: 1},
		{name: "different project", project: "other_project", workspace: workspace.ID},
		{name: "different workspace", project: workspace.ProjectID, workspace: "other_workspace"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.Events(ctx, tt.project, tt.workspace)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != tt.want {
				t.Fatalf("got %d events, want %d", len(got), tt.want)
			}
			if tt.want == 1 && got[0].ID != event.ID {
				t.Fatalf("got event %q, want %q", got[0].ID, event.ID)
			}
		})
	}
}
