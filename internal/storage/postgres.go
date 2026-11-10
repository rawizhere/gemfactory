package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"log/slog"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

type Postgres struct {
	db *bun.DB
}

func NewPostgres(ctx context.Context, databaseURL string, logger *slog.Logger) (*Postgres, error) {
	const maxRetries = 10

	var db *bun.DB
	var sqldb *sql.DB
	var err error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				err = ctx.Err()
			case <-time.After(5 * time.Second):
			}
			if err != nil {
				break
			}
		}
		sqldb = sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(databaseURL)))
		sqldb.SetMaxOpenConns(25)
		sqldb.SetMaxIdleConns(10)
		sqldb.SetConnMaxLifetime(5 * time.Minute)
		sqldb.SetConnMaxIdleTime(1 * time.Minute)

		candidate := bun.NewDB(sqldb, pgdialect.New())

		initCtx, initCancel := context.WithTimeout(ctx, 5*time.Second)
		_, err = candidate.ExecContext(initCtx, "SET search_path TO gemfactory, public")
		initCancel()
		if err != nil {
			logger.Warn("Failed to set search_path", slog.Any("error", err))
		}

		if logger.Enabled(context.Background(), slog.LevelDebug) {
			candidate.AddQueryHook(debugQueryHook{logger: logger})
		}

		pingCtx, pingCancel := context.WithTimeout(ctx, 10*time.Second)
		pingErr := candidate.PingContext(pingCtx)
		pingCancel()

		if pingErr != nil {
			_ = candidate.Close()
			err = pingErr
			logger.Warn("Failed to connect to database", slog.Int("attempt", attempt+1), slog.Any("error", err))
			continue
		}

		db = candidate
		err = nil
		break
	}
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	logger.Info("Connected to PostgreSQL database with Bun ORM")

	if err := Migrate(ctx, sqldb, logger); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &Postgres{db: db}, nil
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

// debugQueryHook logs every SQL statement at debug level.
type debugQueryHook struct {
	logger *slog.Logger
}

func (h debugQueryHook) BeforeQuery(ctx context.Context, _ *bun.QueryEvent) context.Context {
	return ctx
}

func (h debugQueryHook) AfterQuery(_ context.Context, event *bun.QueryEvent) {
	attrs := []any{slog.String("query", event.Query), slog.Duration("duration", time.Since(event.StartTime))}
	if event.Err != nil {
		attrs = append(attrs, slog.Any("error", event.Err))
	}
	h.logger.Debug("SQL", attrs...)
}
