// Package storage provides the database infrastructure and repository implementations.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"gemfactory/internal/model"
	"gemfactory/internal/storage/repository"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/extra/bundebug"
	"go.uber.org/zap"
)

// Postgres encapsulates a PostgreSQL database connection managed via the Bun ORM.
type Postgres struct {
	db     *bun.DB
	logger *zap.Logger
}

// NewPostgres initializes a new PostgreSQL connection with a robust retry mechanism.
func NewPostgres(ctx context.Context, databaseURL string, logger *zap.Logger) (*Postgres, error) {
	const maxRetries = 10
	const retryDelay = 5 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Check context before each attempt.
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("connection cancelled: %w", ctx.Err())
		default:
		}

		logger.Info("Attempting to connect to database",
			zap.Int("attempt", attempt),
			zap.Int("max_retries", maxRetries))

		// Initialize PostgreSQL connector.
		sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(databaseURL)))

		// Configure connection pool.
		sqldb.SetMaxOpenConns(25)
		sqldb.SetMaxIdleConns(10)
		sqldb.SetConnMaxLifetime(5 * time.Minute)
		sqldb.SetConnMaxIdleTime(1 * time.Minute)

		// Initialize Bun DB.
		db := bun.NewDB(sqldb, pgdialect.New())

		// Set default search_path.
		initCtx, initCancel := context.WithTimeout(ctx, 5*time.Second)
		_, err := db.ExecContext(initCtx, "SET search_path TO gemfactory, public")
		initCancel()
		if err != nil {
			logger.Warn("Failed to set search_path", zap.Error(err))
		}

		// Enable debug hooks in debug mode.
		if logger.Core().Enabled(zap.DebugLevel) {
			db.AddQueryHook(bundebug.NewQueryHook(
				bundebug.WithVerbose(true),
				bundebug.FromEnv("BUNDEBUG"),
			))
		}

		// Verify connection.
		pingCtx, pingCancel := context.WithTimeout(ctx, 10*time.Second)
		pingErr := db.PingContext(pingCtx)
		pingCancel()

		if pingErr != nil {
			logger.Warn("Failed to connect to database",
				zap.Int("attempt", attempt),
				zap.Error(pingErr))

			// Cleanup failed connection attempt.
			if err := db.Close(); err != nil {
				logger.Warn("Failed to close database connection", zap.Error(err))
			}

			if attempt == maxRetries {
				return nil, fmt.Errorf("failed to connect to database after %d attempts: %w", maxRetries, pingErr)
			}

			logger.Info("Retrying connection",
				zap.Duration("delay", retryDelay))

			// Wait before retry, respecting context.
			select {
			case <-time.After(retryDelay):
				continue
			case <-ctx.Done():
				return nil, fmt.Errorf("interrupted during retry delay: %w", ctx.Err())
			}
		}

		logger.Info("Connected to PostgreSQL database with Bun ORM",
			zap.Int("attempt", attempt))

		return &Postgres{
			db:     db,
			logger: logger,
		}, nil
	}

	return nil, fmt.Errorf("unexpected error: max retries exceeded")
}

// Close terminates the underlying database connection.
func (p *Postgres) Close() error {
	return p.db.Close()
}

// GetDB returns the underlying Bun database instance.
func (p *Postgres) GetDB() *bun.DB {
	return p.db
}

// GetArtistRepository returns an implementation of the Artist repository.
func (p *Postgres) GetArtistRepository() model.ArtistRepository {
	return repository.NewArtistRepository(p.db, p.logger)
}

// GetReleaseRepository returns an implementation of the Release repository.
func (p *Postgres) GetReleaseRepository() model.ReleaseRepository {
	return repository.NewReleaseRepository(p.db, p.logger)
}

// GetHomeworkRepository returns an implementation of the Homework repository.
func (p *Postgres) GetHomeworkRepository() model.HomeworkRepository {
	return repository.NewHomeworkRepository(p.db, p.logger)
}

// GetPlaylistTracksRepository returns an implementation of the PlaylistTracks repository.
func (p *Postgres) GetPlaylistTracksRepository() model.PlaylistTracksRepository {
	return repository.NewPlaylistTracksRepository(p.db, p.logger)
}

// GetConfigRepository returns an implementation of the Config repository.
func (p *Postgres) GetConfigRepository() model.ConfigRepository {
	return repository.NewConfigRepository(p.db, p.logger)
}

// Ping verifies that the database connection is still active.
func (p *Postgres) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return p.db.PingContext(ctx)
}

// Query executes a raw SQL query and returns the matching rows.
func (p *Postgres) Query(query string, args ...interface{}) (*sql.Rows, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return p.db.QueryContext(ctx, query, args...)
}
