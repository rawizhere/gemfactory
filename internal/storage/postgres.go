package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/extra/bundebug"
	"go.uber.org/zap"
)

type Postgres struct {
	db     *bun.DB
	logger *zap.Logger
}

func NewPostgres(ctx context.Context, databaseURL string, logger *zap.Logger) (*Postgres, error) {
	const maxRetries = 10
	const retryDelay = 5 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("connection cancelled: %w", ctx.Err())
		default:
		}

		sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(databaseURL)))
		sqldb.SetMaxOpenConns(25)
		sqldb.SetMaxIdleConns(10)
		sqldb.SetConnMaxLifetime(5 * time.Minute)
		sqldb.SetConnMaxIdleTime(1 * time.Minute)

		db := bun.NewDB(sqldb, pgdialect.New())

		initCtx, initCancel := context.WithTimeout(ctx, 5*time.Second)
		_, err := db.ExecContext(initCtx, "SET search_path TO gemfactory, public")
		initCancel()
		if err != nil {
			logger.Warn("Failed to set search_path", zap.Error(err))
		}

		if logger.Core().Enabled(zap.DebugLevel) {
			db.AddQueryHook(bundebug.NewQueryHook(
				bundebug.WithVerbose(true),
				bundebug.FromEnv("BUNDEBUG"),
			))
		}

		pingCtx, pingCancel := context.WithTimeout(ctx, 10*time.Second)
		pingErr := db.PingContext(pingCtx)
		pingCancel()

		if pingErr != nil {
			logger.Warn("Failed to connect to database", zap.Int("attempt", attempt), zap.Error(pingErr))
			_ = db.Close()

			if attempt == maxRetries {
				return nil, fmt.Errorf("failed to connect to database after %d attempts: %w", maxRetries, pingErr)
			}

			select {
			case <-time.After(retryDelay):
				continue
			case <-ctx.Done():
				return nil, fmt.Errorf("interrupted during retry delay: %w", ctx.Err())
			}
		}

		logger.Info("Connected to PostgreSQL database with Bun ORM")
		return &Postgres{
			db:     db,
			logger: logger,
		}, nil
	}

	return nil, fmt.Errorf("unexpected error: max retries exceeded")
}

func (p *Postgres) Close() error {
	return p.db.Close()
}

func (p *Postgres) GetDB() *bun.DB {
	return p.db
}

func (p *Postgres) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return p.db.PingContext(ctx)
}
