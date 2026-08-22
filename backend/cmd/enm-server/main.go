package main

import (
	"log"
	"net/http"
	"os"

	"event-network-manager/backend/internal/api"
	"event-network-manager/backend/internal/platform"
)

func main() {
	services := platform.NewServicesFromEnvironment()
	handler := api.NewHandler(services)
	addr := env("ENM_ADDR", "127.0.0.1:8787")
	log.Printf("Event Network Manager API listening on http://%s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal(err)
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
