package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
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

	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("cannot initialize logger: %v", err)
	}
	defer logger.Sync()

	metricsServer := handler.NewMetricsServer(stor, logger)

	srv := &http.Server{
		Addr:         *addr,
		Handler:      metricsServer.Routes(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("Starting metrics server on %s", *addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	stor.Close()

	log.Println("Server exited properly")
}