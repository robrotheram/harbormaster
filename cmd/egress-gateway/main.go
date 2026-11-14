package main

import (
	"agent-workbench/internal/egress"
	"agent-workbench/internal/gateway"
	"agent-workbench/internal/shared"
	"log"
)

func main() {
	token, err := shared.Secret("GATEWAY_SERVICE_TOKEN")
	if err != nil {
		log.Fatal("gateway authority unavailable")
	}
	verify, err := gateway.NewVerifier(shared.Env("CONTROLLER_URL", "http://runtime-controller:8080"), token)
	if err != nil {
		log.Fatal("invalid verifier")
	}
	if err := shared.Serve(":8080", egress.Handler(verify, "", "")); err != nil {
		log.Fatal("egress server stopped")
	}
}
