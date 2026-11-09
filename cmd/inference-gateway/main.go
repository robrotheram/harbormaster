package main

import (
	"agent-workbench/internal/gateway"
	"context"
	"log"
	"net/http"
	"time"
)

func main() {
	c, err := gateway.ConfigFromEnv()
	if err != nil {
		log.Fatal("invalid gateway configuration")
	}
	h, err := gateway.NewHandler(c)
	if err != nil {
		log.Fatal("invalid gateway configuration")
	}
	go func() {
		if gateway.MaintainSubscriptionLease(context.Background(), c) != nil {
			log.Print("subscription Vault lease maintenance unavailable")
		}
	}()
	s := &http.Server{Addr: ":8080", Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 40 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	if err := s.ListenAndServe(); err != nil {
		log.Fatal("gateway server stopped")
	}
}
