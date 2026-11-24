package shared

import "context"

type workspaceQuotaUserKey struct{}

func WithWorkspaceQuotaUser(ctx context.Context, user string) context.Context {
	if user == "" {
		return ctx
	}
	return context.WithValue(ctx, workspaceQuotaUserKey{}, user)
}

func WorkspaceQuotaUser(ctx context.Context) string {
	user, _ := ctx.Value(workspaceQuotaUserKey{}).(string)
	return user
}
