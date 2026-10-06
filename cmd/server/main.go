package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"metrics-alerting/internal/handler"
	"metrics-alerting/internal/storage"
)

func main() {
	addr := flag.String("a", "localhost:8080", "HTTP server address")
	flag.Parse()

	if envAddr := os.Getenv("ADDRESS"); envAddr != "" {
		*addr = envAddr
	}

	stor := storage.NewMemStorage()
	metricsServer := handler.NewMetricsServer(stor)

	log.Printf("Starting metrics server on %s", *addr)
	if err := http.ListenAndServe(*addr, metricsServer.Routes()); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}