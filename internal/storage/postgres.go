package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/extra/bundebug"
	"go.uber.org/zap"
)

type Postgres struct {
	db     *bun.DB
	sqldb  *sql.DB
	logger *zap.Logger
}

func NewPostgres(ctx context.Context, databaseURL string, logger *zap.Logger) (*Postgres, error) {
	const maxRetries = 10

	var db *bun.DB
	var sqldb *sql.DB
	err := retry.Do(
		func() error {
			sqldb = sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(databaseURL)))
			sqldb.SetMaxOpenConns(25)
			sqldb.SetMaxIdleConns(10)
			sqldb.SetConnMaxLifetime(5 * time.Minute)
			sqldb.SetConnMaxIdleTime(1 * time.Minute)

			candidate := bun.NewDB(sqldb, pgdialect.New())

			initCtx, initCancel := context.WithTimeout(ctx, 5*time.Second)
			_, err := candidate.ExecContext(initCtx, "SET search_path TO gemfactory, public")
			initCancel()
			if err != nil {
				logger.Warn("Failed to set search_path", zap.Error(err))
			}

			if logger.Core().Enabled(zap.DebugLevel) {
				candidate.AddQueryHook(bundebug.NewQueryHook(
					bundebug.WithVerbose(true),
					bundebug.FromEnv("BUNDEBUG"),
				))
			}

			pingCtx, pingCancel := context.WithTimeout(ctx, 10*time.Second)
			pingErr := candidate.PingContext(pingCtx)
			pingCancel()

			if pingErr != nil {
				_ = candidate.Close()
				return pingErr
			}

			db = candidate
			return nil
		},
		retry.Context(ctx),
		retry.Attempts(maxRetries),
		retry.Delay(5*time.Second),
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			logger.Warn("Failed to connect to database", zap.Uint("attempt", n+1), zap.Error(err))
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	logger.Info("Connected to PostgreSQL database with Bun ORM")

	if err := Migrate(ctx, sqldb, logger); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &Postgres{
		db:     db,
		sqldb:  sqldb,
		logger: logger,
	}, nil
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
