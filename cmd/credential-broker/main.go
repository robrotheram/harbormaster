package main

import (
	"agent-workbench/internal/broker"
	"log"
	"net/http"
	"time"
)

func main() {
	c, err := broker.ConfigFromEnv()
	if err != nil {
		log.Fatal("invalid broker configuration")
	}
	h, err := broker.NewHandler(c)
	if err != nil {
		log.Fatal("invalid broker configuration")
	}
	s := &http.Server{Addr: ":8080", Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 40 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	if err := s.ListenAndServe(); err != nil {
		log.Fatal("broker server stopped")
	}
}
