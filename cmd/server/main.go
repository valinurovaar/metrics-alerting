package main

import (
	"log"
	"net/http"

	"metrics-alerting/internal/handler"
	"metrics-alerting/internal/storage"
)

func main() {
	storage := storage.NewMemStorage()

	metricsServer := handler.NewMetricsServer(storage)

	http.HandleFunc("/update/", metricsServer.UpdateHandler)

	addr := "localhost:8080"
	log.Printf("Starting metrics server on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
