package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"go.uber.org/zap"

	"metrics-alerting/internal/handler"
	"metrics-alerting/internal/storage"
)

func main() {
	addr := flag.String("a", "localhost:8080", "HTTP server address")

	storeInterval := flag.Int(
		"i",
		300,
		"Store interval in seconds. 0 means synchronous write after every update",
	)

	fileStoragePath := flag.String(
		"f",
		"metrics.json",
		"File storage path",
	)

	restore := flag.Bool(
		"r",
		false,
		"Restore saved metrics from file on start",
	)

	flag.Parse()

	if envAddr := os.Getenv("ADDRESS"); envAddr != "" {
		*addr = envAddr
	}

	if envStoreInterval := os.Getenv("STORE_INTERVAL"); envStoreInterval != "" {
		if value, err := strconv.Atoi(envStoreInterval); err == nil {
			*storeInterval = value
		} else {
			log.Printf(
				"invalid STORE_INTERVAL %q, using current value %d",
				envStoreInterval,
				*storeInterval,
			)
		}
	}

	if envFileStoragePath := os.Getenv("FILE_STORAGE_PATH"); envFileStoragePath != "" {
		*fileStoragePath = envFileStoragePath
	}

	if envRestore := os.Getenv("RESTORE"); envRestore != "" {
		if value, err := strconv.ParseBool(envRestore); err == nil {
			*restore = value
		} else {
			log.Printf(
				"invalid RESTORE %q, using current value %t",
				envRestore,
				*restore,
			)
		}
	}

	stor, err := storage.NewPersistentMemStorage(storage.PersistenceConfig{
		FilePath:      *fileStoragePath,
		Restore:       *restore,
		StoreInterval: time.Duration(*storeInterval) * time.Second,
	})
	if err != nil {
		log.Fatalf("cannot initialize storage: %v", err)
	}
	defer stor.Close()

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