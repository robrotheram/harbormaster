// Package operator implements the supported administrative CLI. It is not used
// to render or reconcile Kubernetes resources.
package operator

import (
	"context"
	"errors"
	"io"
)

func Run(ctx context.Context, args []string, out, errOut io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: workbench bootstrap|workspace|project|smoke|check-runtime-host")
	}
	switch args[0] {
	case "bootstrap":
		if len(args) != 1 {
			return errors.New("bootstrap takes no arguments")
		}
		return Bootstrap(".", out)
	case "workspace", "project":
		return APICommand(ctx, args[0], args[1:], out, errOut)
	case "smoke":
		return Smoke(ctx, args[1:], out, errOut)
	case "check-runtime-host":
		return CheckRuntimeHost(ctx, out, errOut)
	default:
		return errors.New("unknown operator command")
	}
}
