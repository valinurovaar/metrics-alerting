package migrations

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/lib/pq"
)

//go:embed *.sql
var migrationFiles embed.FS

func Up(dsn string) error {
	database, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("open migration database: %w", err)
	}
	defer database.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := database.PingContext(ctx); err != nil {
		return fmt.Errorf("ping migration database: %w", err)
	}

	sourceDriver, err := iofs.New(migrationFiles, ".")
	if err != nil {
		return fmt.Errorf("create migration source: %w", err)
	}

	databaseDriver, err := postgres.WithInstance(database, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("create migration database driver: %w", err)
	}

	migrator, err := migrate.NewWithInstance("iofs", sourceDriver, "postgres", databaseDriver)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}

	upErr := migrator.Up()
	if errors.Is(upErr, migrate.ErrNoChange) {
		upErr = nil
	}
	sourceErr, databaseErr := migrator.Close()

	if err := errors.Join(upErr, sourceErr, databaseErr); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
