package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"go.uber.org/zap"

	"metrics-alerting/internal/handler"
	"metrics-alerting/internal/storage"
	"metrics-alerting/migrations"
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
		"",
		"File storage path",
	)

	restore := flag.Bool(
		"r",
		false,
		"Restore saved metrics from file on start",
	)
	databaseDSN := flag.String("d", "", "PostgreSQL database connection string")

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

	if envDatabaseDSN := os.Getenv("DATABASE_DSN"); envDatabaseDSN != "" {
		*databaseDSN = envDatabaseDSN
	}

	var (
		database *sql.DB
		stor     storage.Storage
		memStore *storage.MemStorage
	)

	if *databaseDSN != "" {
		var err error
		database, err = sql.Open("postgres", *databaseDSN)
		if err != nil {
			log.Fatalf("cannot initialize database connection: %v", err)
		}

		if err := migrations.Up(*databaseDSN); err != nil {
			log.Fatalf("cannot apply database migrations: %v", err)
		}

		stor = storage.NewPostgresStorage(database)
	} else if *fileStoragePath != "" {
		var err error
		memStore, err = storage.NewPersistentMemStorage(storage.PersistenceConfig{
			FilePath:      *fileStoragePath,
			Restore:       *restore,
			StoreInterval: time.Duration(*storeInterval) * time.Second,
		})
		if err != nil {
			log.Fatalf("cannot initialize file storage: %v", err)
		}
		stor = memStore
	} else {
		memStore = storage.NewMemStorage()
		stor = memStore
	}

	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("cannot initialize logger: %v", err)
	}
	defer logger.Sync()

	metricsServer := handler.NewMetricsServer(stor, logger)
	if database != nil {
		metricsServer = handler.NewMetricsServer(stor, logger, database)
	}
	requestContext, cancelRequests := context.WithCancel(context.Background())
	defer cancelRequests()

	srv := &http.Server{
		Addr:    *addr,
		Handler: metricsServer.Routes(),
		BaseContext: func(net.Listener) context.Context {
			return requestContext
		},
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
	cancelRequests()

	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()

	if err := srv.Shutdown(shutdownContext); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	if memStore != nil {
		memStore.Close()
	}
	if database != nil {
		if err := database.Close(); err != nil {
			log.Printf("cannot close database connection: %v", err)
		}
	}

	log.Println("Server exited properly")
}
