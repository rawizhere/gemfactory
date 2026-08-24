package storage

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strconv"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"go.uber.org/zap"

	"gemfactory/migrations"
)

// migrationsTableName tracks applied migration versions.
const migrationsTableName = "schema_migrations"

// schemaSentinelTable is a table whose presence indicates the database was
// initialized before version tracking was introduced (via psql in start.sh).
const schemaSentinelTable = "artists"

var migrationFileRe = regexp.MustCompile(`^0*(\d+)_.*\.up\.sql$`)

// Migrate applies pending migrations using golang-migrate over the embedded SQL
// files (NNNN_name.up.sql / .down.sql pairs). Databases initialized by the
// legacy start.sh flow (schema exists but no version table) are baselined at
// the latest embedded migration instead of re-running everything.
func Migrate(ctx context.Context, sqldb *sql.DB, logger *zap.Logger) error {
	// Migration 000001 also creates the schema, but the migrator needs it first.
	if _, err := sqldb.ExecContext(ctx, `CREATE SCHEMA IF NOT EXISTS gemfactory`); err != nil {
		return fmt.Errorf("failed to ensure app schema: %w", err)
	}

	if err := baselineIfNeeded(ctx, sqldb, logger); err != nil {
		return fmt.Errorf("migration baseline failed: %w", err)
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
	m.Log = &zapMigrateLogger{logger: logger}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrations failed: %w", err)
	}

	version, dirty, err := dbDrv.Version()
	if err != nil {
		return fmt.Errorf("failed to read migration version: %w", err)
	}
	logger.Info("Database migrations up to date",
		zap.Int("version", int(version)), zap.Bool("dirty", dirty))
	return nil
}

// baselineIfNeeded marks an existing unversioned database as fully migrated.
func baselineIfNeeded(ctx context.Context, sqldb *sql.DB, logger *zap.Logger) error {
	versioned, err := tableExists(ctx, sqldb, migrationsTableName)
	if err != nil {
		return err
	}
	if versioned {
		return nil
	}

	initialized, err := tableExists(ctx, sqldb, schemaSentinelTable)
	if err != nil {
		return err
	}
	if !initialized {
		return nil // fresh database: let the migrator apply from scratch
	}

	latest, err := latestEmbeddedVersion()
	if err != nil {
		return err
	}

	logger.Info("Existing unversioned database detected, baselining migrations",
		zap.Int64("version", latest))

	baselineSQL := fmt.Sprintf(
		`CREATE TABLE gemfactory.%s (
			version BIGINT NOT NULL,
			dirty BOOLEAN NOT NULL
		);
		INSERT INTO gemfactory.%s (version, dirty) VALUES (%d, FALSE);`,
		migrationsTableName, migrationsTableName, latest)

	if _, err := sqldb.ExecContext(ctx, baselineSQL); err != nil {
		return fmt.Errorf("failed to create baseline version table: %w", err)
	}
	return nil
}

// tableExists checks public and gemfactory schemas for the given table.
func tableExists(ctx context.Context, sqldb *sql.DB, table string) (bool, error) {
	var n int
	err := sqldb.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM information_schema.tables
		 WHERE table_schema IN ('gemfactory', 'public') AND table_name = $1`, table).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("failed to check table %q: %w", table, err)
	}
	return n > 0, nil
}

// latestEmbeddedVersion returns the highest NNNN found among embedded *.up.sql files.
func latestEmbeddedVersion() (int64, error) {
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		return 0, fmt.Errorf("failed to list embedded migrations: %w", err)
	}
	var max int64
	for _, e := range entries {
		if m := migrationFileRe.FindStringSubmatch(e.Name()); len(m) == 2 {
			v, err := strconv.ParseInt(m[1], 10, 64)
			if err != nil {
				continue
			}
			if v > max {
				max = v
			}
		}
	}
	if max == 0 {
		return 0, fmt.Errorf("no migration files found")
	}
	return max, nil
}

type zapMigrateLogger struct{ logger *zap.Logger }

func (l *zapMigrateLogger) Printf(format string, v ...interface{}) {
	l.logger.Info(fmt.Sprintf(format, v...))
}
func (l *zapMigrateLogger) Verbose() bool { return l.logger.Core().Enabled(zap.DebugLevel) }
