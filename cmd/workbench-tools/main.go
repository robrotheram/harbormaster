package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"agent-workbench/internal/workspacetools"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	name, args := filepath.Base(os.Args[0]), os.Args[1:]
	if name == "workbench-tools" {
		if len(args) == 0 {
			fmt.Fprintln(os.Stderr, "usage: workbench-tools preview|desktop|research|gogallery|gallery-fixture")
			os.Exit(2)
		}
		name, args = args[0], args[1:]
	}
	if err := workspacetools.Run(ctx, name, args, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
