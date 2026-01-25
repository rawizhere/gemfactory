// Package app coordinates the lifecycle and components of the Telegram bot.
package app

import (
	"context"
	"fmt"
	"gemfactory/internal/config"
	"gemfactory/internal/health"
	"gemfactory/internal/middleware"
	"gemfactory/internal/service"
	"gemfactory/internal/storage"
	"gemfactory/internal/telegram"
	"gemfactory/internal/worker"
	"gemfactory/pkg/logger"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Bot maintains the state and orchestration for the bot's core systems.
type Bot struct {
	config        *config.Config
	logger        *zap.Logger
	db            *storage.Postgres
	telegram      *telegram.Client
	health        *health.Server
	services      *service.Services
	middleware    *middleware.Middleware
	workerManager *worker.Manager
	stopChan      chan struct{}
	wg            sync.WaitGroup
	ctx           context.Context
	cancel        context.CancelFunc
	loggerWrapper *logger.Logger
}

// NewBot initializes a new Bot instance with basic configuration.
func NewBot(cfg *config.Config, logger *zap.Logger) (*Bot, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	// Create context for lifecycle management
	ctx, cancel := context.WithCancel(context.Background())

	bot := &Bot{
		config:   cfg,
		logger:   logger,
		stopChan: make(chan struct{}),
		ctx:      ctx,
		cancel:   cancel,
	}

	logger.Info("Bot structure created successfully")
	return bot, nil
}

// NewBotWithFactory creates a new Bot instance using a component factory.
func NewBotWithFactory(ctx context.Context, cfg *config.Config, l *logger.Logger) (*Bot, error) {
	factory, err := NewComponentFactory(ctx, cfg, l)
	if err != nil {
		return nil, fmt.Errorf("failed to create component factory: %w", err)
	}
	return factory.CreateBot(ctx)
}

// Start initiates all background services and the main update processing loop.
func (b *Bot) Start(ctx context.Context) error {
	b.logger.Info("Starting bot")

	if b.health != nil {
		b.wg.Add(1)
		go func() {
			defer b.wg.Done()
			select {
			case <-b.ctx.Done():
				b.logger.Info("Health check server cancelled by context")
				return
			default:
				if err := b.health.Start(); err != nil {
					if err.Error() == "http: Server closed" {
						b.logger.Info("Health check server stopped normally")
					} else {
						b.logger.Error("Health check server failed", zap.Error(err))
					}
				}
			}
		}()
	}

	// Start middleware cleanup with context
	if b.middleware != nil {
		b.wg.Add(1)
		go func() {
			defer b.wg.Done()
			ticker := time.NewTicker(5 * time.Minute) // Cleanup every 5 minutes
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					b.middleware.Cleanup()
				case <-b.ctx.Done():
					b.logger.Info("Middleware cleanup stopped by context")
					return
				case <-b.stopChan:
					b.logger.Info("Middleware cleanup stopped by stop signal")
					return
				}
			}
		}()
	}

	b.logger.Info("Bot started successfully")

	// Load playlist on startup
	if b.services.Playlist != nil {
		b.wg.Add(1)
		go func() {
			defer b.wg.Done()
			b.logger.Info("Loading playlist on startup...")
			// Update playlist
			if err := b.services.Playlist.Reload(ctx); err != nil {
				b.logger.Error("Failed to load playlist on startup", zap.Error(err))
			} else {
				b.logger.Info("Playlist loaded successfully on startup")
			}
		}()
	} else {
		b.logger.Warn("Playlist service not available, skipping initial playlist load")
	}

	// Start configuration watcher
	if b.services.ConfigWatcher != nil {
		b.wg.Add(1)
		go func() {
			defer b.wg.Done()
			b.services.ConfigWatcher.Start(b.ctx)
		}()
		b.logger.Info("Config watcher started successfully")
	}

	// Start background workers
	if b.workerManager != nil {
		b.workerManager.Start(b.ctx)
	}

	// Main update processing loop
	maxRestartAttempts := 10
	restartAttempts := 0
	restartDelay := 10 * time.Second

	for {
		select {
		case <-b.ctx.Done():
			b.logger.Info("Bot main loop cancelled by context")
			return b.ctx.Err()
		case <-b.stopChan:
			b.logger.Info("Bot main loop stopped by stop signal")
			return nil
		default:
			if err := b.runUpdateLoop(ctx); err != nil {
				if err.Error() == "context canceled" || err == context.Canceled {
					b.logger.Info("Update loop stopped due to context cancellation")
					return err
				}

				restartAttempts++
				b.logger.Error("Update loop error",
					zap.Error(err),
					zap.Int("restart_attempt", restartAttempts),
					zap.Int("max_attempts", maxRestartAttempts))

				if restartAttempts > maxRestartAttempts {
					b.logger.Fatal("Max restart attempts reached, bot is shutting down")
					return fmt.Errorf("max restart attempts reached: %w", err)
				}

				delay := time.Duration(restartAttempts) * restartDelay
				if delay > 5*time.Minute {
					delay = 5 * time.Minute
				}

				b.logger.Info("Waiting before restart", zap.Duration("delay", delay))
				select {
				case <-b.ctx.Done():
					return b.ctx.Err()
				case <-time.After(delay):
					continue
				}
			} else {
				restartAttempts = 0
			}
		}
	}
}

// Stop performs a graceful shutdown of all bot services and connections.
func (b *Bot) Stop() error {
	b.logger.Info("Stopping bot gracefully")

	// Stop configuration watcher
	if b.services.ConfigWatcher != nil {
		b.logger.Info("Stopping config watcher")
		b.services.ConfigWatcher.Stop()
	}

	// Cancel context to stop all goroutines
	if b.cancel != nil {
		b.logger.Debug("Cancelling bot context")
		b.cancel()
	}

	// Send stop signal (for backward compatibility)
	select {
	case <-b.stopChan:
		b.logger.Debug("Stop channel already closed")
	default:
		b.logger.Debug("Closing stop channel")
		close(b.stopChan)
	}

	// Create context with timeout for graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	b.logger.Debug("Graceful shutdown timeout set", zap.Duration("timeout", 30*time.Second))

	// Stop health check server with context
	if b.health != nil {
		b.logger.Debug("Stopping health check server")
		if err := b.health.Stop(); err != nil {
			b.logger.Error("Failed to stop health check server", zap.Error(err))
		} else {
			b.logger.Debug("Health check server stopped successfully")
		}
	}

	// Wait for all goroutines to complete with timeout
	done := make(chan struct{})
	go func() {
		defer close(done)
		b.logger.Debug("Waiting for all goroutines to complete")
		b.wg.Wait()
	}()

	select {
	case <-done:
		b.logger.Info("All goroutines stopped successfully")
	case <-shutdownCtx.Done():
		b.logger.Warn("Graceful shutdown timeout exceeded, forcing stop")
	}

	// Close database connection
	if err := b.db.Close(); err != nil {
		b.logger.Error("Failed to close database connection", zap.Error(err))
	}

	b.logger.Info("Bot stopped successfully")
	return nil
}

// runUpdateLoop initializes the router and begins fetching updates from Telegram.
func (b *Bot) runUpdateLoop(ctx context.Context) error {
	b.logger.Info("Starting update loop")

	// Create router
	router := NewRouterWithBotAPI(b.services, b.config, b.logger, b.telegram.GetBotAPI())

	return b.telegram.Start(ctx, router)
}
