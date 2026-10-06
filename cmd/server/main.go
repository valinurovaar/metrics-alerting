package main

import (
	"log"
	"net/http"

	"metrics-alerting/internal/handler"
	"metrics-alerting/internal/storage"
)

func main() {
	stor := storage.NewMemStorage()
	metricsServer := handler.NewMetricsServer(stor)

	addr := "localhost:8080"
	log.Printf("Starting metrics server on %s", addr)
	if err := http.ListenAndServe(addr, metricsServer.Routes()); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
