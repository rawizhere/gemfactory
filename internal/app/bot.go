// Package app coordinates the lifecycle and components of the Telegram bot.
package app

import (
	"context"
	"fmt"
	"gemfactory/internal/config"
	"gemfactory/internal/health"
	"gemfactory/internal/keyboard"
	"gemfactory/internal/middleware"
	"gemfactory/internal/service"
	"gemfactory/internal/storage"
	"gemfactory/internal/telegram"
	"gemfactory/internal/worker"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Bot coordinates the lifecycle of all services, workers, and the Telegram client.
type Bot struct {
	config         *config.Config
	logger         *zap.Logger
	db             *storage.Postgres
	telegram       *telegram.Client
	health         *health.Server
	services       *service.Services
	middleware     *middleware.Middleware
	keyboard       *keyboard.Manager
	router         *Router
	releaseChecker *worker.ReleaseChecker
	wg             sync.WaitGroup
	ctx            context.Context
	cancel         context.CancelFunc
}

// NewBot initializes all subsystems and returns a ready-to-run Bot instance.
func NewBot(ctx context.Context, cfg *config.Config, logger *zap.Logger) (*Bot, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	if err := os.MkdirAll(cfg.AppDataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create app data directory: %w", err)
	}

	db, err := storage.NewPostgres(ctx, cfg.DatabaseURL, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	services := service.NewServices(db, cfg, logger)
	tgClient, err := telegram.NewClient(cfg.BotToken, logger)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to create telegram client: %w", err)
	}

	keyboardManager := keyboard.NewManager(services.Release, cfg, logger)
	keyboardManager.SetTelegramClient(tgClient)

	router := NewRouter(services, cfg, keyboardManager, logger, tgClient)

	var healthServer *health.Server
	if cfg.HealthCheckEnabled {
		healthServer = health.NewServer(cfg.HealthPort, logger, db)
	}

	releaseChecker := worker.NewReleaseChecker(services.Release, logger, cfg.ReleaseCheckInterval)

	botCtx, botCancel := context.WithCancel(ctx)

	return &Bot{
		config:         cfg,
		logger:         logger,
		db:             db,
		telegram:       tgClient,
		health:         healthServer,
		services:       services,
		middleware:     middleware.New(cfg, logger),
		keyboard:       keyboardManager,
		router:         router,
		releaseChecker: releaseChecker,
		ctx:            botCtx,
		cancel:         botCancel,
	}, nil
}

// Start launches the health server, periodic cleanup, background release checker, and long-polling.
func (b *Bot) Start(ctx context.Context) error {
	b.logger.Info("Starting bot")

	if b.health != nil {
		b.wg.Add(1)
		go func() {
			defer b.wg.Done()
			if err := b.health.Start(); err != nil && err.Error() != "http: Server closed" {
				b.logger.Error("Health check server failed", zap.Error(err))
			}
		}()
	}

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				b.middleware.Cleanup()
			case <-b.ctx.Done():
				return
			}
		}
	}()

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		b.releaseChecker.Start(b.ctx)
	}()

	b.logger.Info("Bot started successfully")

	return b.telegram.Start(ctx, b.router)
}

// Stop gracefully shuts down background workers, servers, and database connections.
func (b *Bot) Stop() error {
	b.logger.Info("Stopping bot gracefully")

	if b.cancel != nil {
		b.cancel()
	}

	if b.keyboard != nil {
		b.keyboard.Stop()
	}

	if b.health != nil {
		_ = b.health.Stop()
	}

	stopped := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(stopped)
	}()

	select {
	case <-stopped:
		b.logger.Info("All background workers stopped cleanly")
	case <-time.After(15 * time.Second):
		b.logger.Warn("Shutdown timed out waiting for background workers")
	}

	if b.db != nil {
		_ = b.db.Close()
	}

	b.logger.Info("Bot stopped successfully")
	return nil
}
