// Package workspacetools contains compiled workspace utilities. These commands
// run inside the workspace boundary and receive only its scoped credentials.
package workspacetools

import (
	"context"
	"fmt"
	"io"
)

func Run(ctx context.Context, name string, args []string, out, errOut io.Writer) error {
	switch name {
	case "hermes-runner", "hermes-matrix", "hermes-digest", "hermes-consume":
		return Hermes(name, args)
	case "preview", "workbench-preview":
		return Preview(ctx, args, out, errOut)
	case "desktop", "workbench-desktop":
		return Desktop(ctx, args, errOut)
	default:
		return fmt.Errorf("unknown workspace tool %q", name)
	}
}
