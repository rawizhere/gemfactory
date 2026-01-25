// Package app provides a factory for assembling and configuring application components.
package app

import (
	"context"
	"fmt"
	"gemfactory/internal/config"
	"gemfactory/internal/health"
	"gemfactory/internal/middleware"
	"gemfactory/internal/model"
	"gemfactory/internal/service"
	"gemfactory/internal/storage"
	"gemfactory/internal/telegram"
	"gemfactory/internal/worker"
	"gemfactory/pkg/logger"
	"os"

	"go.uber.org/zap"
)

// ComponentFactory handles the creation and configuration of various bot sub-systems.
type ComponentFactory struct {
	config *config.Config
	logger *logger.Logger
}

// NewComponentFactory initializes a new component factory.
func NewComponentFactory(ctx context.Context, config *config.Config, l *logger.Logger) (*ComponentFactory, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if l == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	return &ComponentFactory{
		config: config,
		logger: l,
	}, nil
}

// CreateDatabase establishes a connection to the PostgreSQL database.
func (f *ComponentFactory) CreateDatabase(ctx context.Context) (*storage.Postgres, error) {
	if f.config.DatabaseURL == "" {
		return nil, fmt.Errorf("database URL is required")
	}

	db, err := storage.NewPostgres(ctx, f.config.DatabaseURL, f.logger.Logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create database connection: %w", err)
	}

	f.logger.Info("Database connection created successfully")
	return db, nil
}

// CreateTelegramClient initializes a Telegram client.
func (f *ComponentFactory) CreateTelegramClient() (*telegram.Client, error) {
	if f.config.BotToken == "" {
		return nil, fmt.Errorf("bot token is required")
	}

	client, err := telegram.NewClient(f.config.BotToken, f.config, f.logger.Logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create telegram client: %w", err)
	}

	f.logger.Info("Telegram client created successfully")
	return client, nil
}

// CreateServices initializes all business logic services with their dependencies.
func (f *ComponentFactory) CreateServices(db *storage.Postgres) (*service.Services, error) {
	if db == nil {
		return nil, fmt.Errorf("database is required")
	}

	services := service.NewServices(db, f.config, f.logger.Logger)
	f.logger.Info("Services created successfully")
	return services, nil
}

// CreateMiddleware initializes the application middleware manager.
func (f *ComponentFactory) CreateMiddleware() *middleware.Middleware {
	middlewareManager := middleware.New(f.config, f.logger.Logger)
	f.logger.Info("Middleware created successfully")
	return middlewareManager
}

// CreateHealthServer initializes a server for monitoring application health.
func (f *ComponentFactory) CreateHealthServer(db *storage.Postgres) (*health.Server, error) {
	if !f.config.HealthCheckEnabled {
		f.logger.Info("Health check server is disabled")
		return nil, nil
	}

	if f.config.HealthPort == "" {
		return nil, fmt.Errorf("health port is required when health check is enabled")
	}

	server := health.NewServer(f.config.HealthPort, f.logger.Logger, db)
	f.logger.Info("Health check server created", zap.String("port", f.config.HealthPort))
	return server, nil
}

// CreateAppDataDirectory ensures the application's data directory exists.
func (f *ComponentFactory) CreateAppDataDirectory() error {
	dataDir := f.config.GetAppDataDir()
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		f.logger.Error("Failed to create app data directory", zap.String("dir", dataDir), zap.Error(err))
		return fmt.Errorf("failed to create app data directory: %w", err)
	}
	f.logger.Info("App data directory ready", zap.String("dir", dataDir))
	return nil
}

// CreateBot assembles a fully initialized Bot instance with all required dependencies.
func (f *ComponentFactory) CreateBot(ctx context.Context) (*Bot, error) {
	// Create app data directory
	if err := f.CreateAppDataDirectory(); err != nil {
		return nil, fmt.Errorf("failed to create app data directory: %w", err)
	}

	// Create database
	db, err := f.CreateDatabase(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create database: %w", err)
	}

	// Create services
	services, err := f.CreateServices(db)
	if err != nil {
		return nil, fmt.Errorf("failed to create services: %w", err)
	}

	// Create Telegram client
	tgClient, err := f.CreateTelegramClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create telegram client: %w", err)
	}

	// Create health check server
	healthServer, err := f.CreateHealthServer(db)
	if err != nil {
		return nil, fmt.Errorf("failed to create health server: %w", err)
	}

	// Create middleware
	middlewareManager := f.CreateMiddleware()

	bot, err := NewBot(f.config, f.logger.Logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot: %w", err)
	}

	// Set components in bot
	bot.db = db
	bot.telegram = tgClient
	bot.health = healthServer
	bot.services = services
	bot.middleware = middlewareManager
	bot.loggerWrapper = f.logger

	// Initialize worker manager
	workerManager := worker.NewManager()
	homeworkWorker := worker.NewHomeworkResetWorker(services.Homework, f.logger.Logger)
	releaseWorker := worker.NewReleaseCheckerWorker(services.Release, f.logger.Logger, f.config.ReleaseCheckInterval)

	workerManager.Add(homeworkWorker)
	workerManager.Add(releaseWorker)
	bot.workerManager = workerManager

	// Register logger and workers as config change subscribers
	if services.ConfigWatcher != nil {
		services.ConfigWatcher.Subscribe(&logConfigurable{f.logger})
		services.ConfigWatcher.Subscribe(releaseWorker)
	}

	f.logger.Info("Bot created successfully with all dependencies")
	return bot, nil
}

// logConfigurable implements Configurable
type logConfigurable struct {
	l *logger.Logger
}

func (lc *logConfigurable) ApplyConfig(ctx context.Context, configs []model.Config) error {
	for _, c := range configs {
		if c.Key == "LOG_LEVEL" {
			lc.l.SetLevel(c.Value)
		}
	}
	return nil
}
