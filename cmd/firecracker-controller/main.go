package main

import (
	"agent-workbench/internal/runtime/firecracker"
	"agent-workbench/internal/shared"
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	token, e := shared.Secret("CONTROLLER_TOKEN")
	if e != nil {
		panic("controller credential unavailable")
	}
	service, e := shared.Secret("GATEWAY_SERVICE_TOKEN")
	if e != nil {
		panic("gateway credential unavailable")
	}
	secret, e := shared.Secret("GRANT_SECRET")
	if e != nil {
		panic("grant secret unavailable")
	}
	f, e := firecracker.NewFirecracker(token, service, secret)
	if e != nil {
		slog.Error("Firecracker admission refused", "error", e)
		os.Exit(1)
	}
	defer f.Close()
	if len(os.Args) == 2 && os.Args[1] == "--self-test" {
		if err := firecracker.FirecrackerSelfTest(f, token, service); err != nil {
			slog.Error("Firecracker acceptance failed", "error", err)
			f.Close()
			os.Exit(1)
		}
		fmt.Println("PASS: jailed VM boot, no-NIC guest checks, admission, grant revocation")
		return
	}
	socket := "/run/agent-workbench/controller.sock"
	if e = os.Remove(socket); e != nil && !os.IsNotExist(e) {
		panic(e)
	}
	l, e := net.Listen("unix", socket)
	if e != nil {
		panic(e)
	}
	defer l.Close()
	if e = os.Chmod(socket, 0600); e != nil {
		panic(e)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if e = shared.ServeListener(ctx, l, f.Handler(), 5*time.Second); e != nil {
		slog.Error("host controller stopped", "error", e)
	}
}
