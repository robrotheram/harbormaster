package orchestration

import (
	"agent-workbench/internal/shared"
	"context"
)

type workActorKey struct{}

func WithWorkActor(ctx context.Context, actor string) context.Context {
	return context.WithValue(ctx, workActorKey{}, actor)
}
func workActorAllowed(ctx context.Context, item shared.WorkItem) bool {
	actor, ok := ctx.Value(workActorKey{}).(string)
	if !ok {
		return true
	}
	if actor == "project-credential" {
		return item.ControllerID == ""
	}
	return actor != "" && item.ControllerID == actor
}
