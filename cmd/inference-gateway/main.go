package main

import (
	"agent-workbench/internal/gateway"
	"agent-workbench/internal/shared"
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	c, err := gateway.ConfigFromEnv()
	if err != nil {
		log.Fatal("invalid gateway configuration")
	}
	h, err := gateway.NewHandler(c)
	if err != nil {
		log.Fatal("invalid gateway configuration")
	}
	go func() {
		if gateway.MaintainSubscriptionLease(ctx, c) != nil {
			log.Print("subscription Vault lease maintenance unavailable")
		}
	}()
	s := &http.Server{Addr: ":8080", Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 40 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	if err := shared.ServeHTTPServer(ctx, s); err != nil {
		log.Fatal("gateway server stopped")
	}
}
