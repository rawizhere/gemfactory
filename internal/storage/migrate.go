package storage

import (
	"context"
	"database/sql"
	"fmt"

	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"gemfactory/migrations"
)

// migrationsTableName tracks applied migration versions.
const migrationsTableName = "schema_migrations"

// Migrate applies pending migrations using golang-migrate over the embedded SQL
// files (NNNN_name.up.sql / .down.sql pairs).
func Migrate(ctx context.Context, sqldb *sql.DB, logger *slog.Logger) error {
	// Migration 000001 also creates the schema, but the migrator needs it first.
	if _, err := sqldb.ExecContext(ctx, `CREATE SCHEMA IF NOT EXISTS gemfactory`); err != nil {
		return fmt.Errorf("failed to ensure app schema: %w", err)
	}

	sourceDrv, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("failed to load embedded migrations: %w", err)
	}
	dbDrv, err := postgres.WithInstance(sqldb, &postgres.Config{
		SchemaName:      "gemfactory",
		MigrationsTable: migrationsTableName,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize migration driver: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", sourceDrv, "postgres", dbDrv)
	if err != nil {
		return fmt.Errorf("failed to initialize migrator: %w", err)
	}
	m.Log = &slogMigrateLogger{logger: logger}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrations failed: %w", err)
	}

	version, dirty, err := dbDrv.Version()
	if err != nil {
		return fmt.Errorf("failed to read migration version: %w", err)
	}
	logger.Info("Database migrations up to date",
		slog.Int("version", int(version)), slog.Bool("dirty", dirty))
	return nil
}

type slogMigrateLogger struct{ logger *slog.Logger }

func (l *slogMigrateLogger) Printf(format string, v ...interface{}) {
	l.logger.Info(fmt.Sprintf(format, v...))
}
func (l *slogMigrateLogger) Verbose() bool {
	return l.logger.Enabled(context.Background(), slog.LevelDebug)
}
