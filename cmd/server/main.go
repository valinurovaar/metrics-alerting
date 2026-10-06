package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"go.uber.org/zap"

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

	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("cannot initialize logger: %v", err)
	}
	defer logger.Sync()

	metricsServer := handler.NewMetricsServer(stor, logger)

	log.Printf("Starting metrics server on %s", *addr)
	if err := http.ListenAndServe(*addr, metricsServer.Routes()); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}