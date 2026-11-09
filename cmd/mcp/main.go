package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"agent-workbench/internal/mcpserver"
)

func main() {
	c, err := mcpserver.ConfigFromEnv()
	if err != nil {
		slog.Error("invalid MCP configuration")
		os.Exit(1)
	}
	h, err := mcpserver.New(c)
	if err != nil {
		slog.Error("invalid MCP configuration")
		os.Exit(1)
	}
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	s := &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 50 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.Shutdown(shutdown)
	}()
	if err = s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("MCP listener failed")
		os.Exit(1)
	}
}
