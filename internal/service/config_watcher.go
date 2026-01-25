// Package service implements the core business logic and domain services.
package service

import (
	"context"
	"gemfactory/internal/config"
	"gemfactory/internal/model"
	"time"

	"go.uber.org/zap"
)

// ConfigWatcher monitors configuration changes and propagates them to interested components.
type ConfigWatcher struct {
	configService ConfigServiceInterface
	logger        *zap.Logger
	stopChan      chan struct{}
	lastHash      string // Last known state hash.
	subscribers   []Configurable
	globalConfig  *config.Config // Reference to the shared config object.
}

// NewConfigWatcher initializes a new configuration monitor.
func NewConfigWatcher(configService ConfigServiceInterface, cfg *config.Config, logger *zap.Logger) *ConfigWatcher {
	return &ConfigWatcher{
		configService: configService,
		logger:        logger,
		stopChan:      make(chan struct{}),
		lastHash:      "",
		subscribers:   make([]Configurable, 0),
		globalConfig:  cfg,
	}
}

// Subscribe registers a component to receive configuration updates.
func (w *ConfigWatcher) Subscribe(subscriber Configurable) {
	w.subscribers = append(w.subscribers, subscriber)
}

// Start begins the polling loop to watch for configuration changes.
func (w *ConfigWatcher) Start(ctx context.Context) {
	w.logger.Info("Starting config watcher")

	ticker := time.NewTicker(60 * time.Second) // Check every 60s.
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("Config watcher stopped due to context cancellation")
			return
		case <-w.stopChan:
			w.logger.Info("Config watcher stopped")
			return
		case <-ticker.C:
			w.checkForConfigChanges(ctx)
		}
	}
}

// Stop terminates the configuration polling loop.
func (w *ConfigWatcher) Stop() {
	close(w.stopChan)
}

func (w *ConfigWatcher) checkForConfigChanges(ctx context.Context) {
	configs, err := w.configService.GetAllRaw(ctx)
	if err != nil {
		w.logger.Error("Failed to get raw configs for watching", zap.Error(err))
		return
	}

	// Calculate the hash of the current state.
	currentHash := ""
	for _, c := range configs {
		currentHash += c.Key + ":" + c.Value + ";"
	}

	if currentHash == w.lastHash {
		return
	}

	w.logger.Info("Config change detected, applying to subscribers and global config",
		zap.Int("subscribers", len(w.subscribers)))

	// 1. Update global config object.
	if err := w.ApplyToGlobalConfig(configs); err != nil {
		w.logger.Error("Failed to update global config", zap.Error(err))
	}

	// 2. Notify subscribers who may need to react to changes (e.g., restart timers).
	for _, sub := range w.subscribers {
		if err := sub.ApplyConfig(ctx, configs); err != nil {
			w.logger.Error("Failed to apply config to subscriber", zap.Error(err))
		}
	}

	w.lastHash = currentHash
}

// ApplyToGlobalConfig synchronizes the dynamic configuration onto the global config object.
func (w *ConfigWatcher) ApplyToGlobalConfig(configs []model.Config) error {
	if w.globalConfig == nil {
		return nil
	}

	for _, c := range configs {
		// Skip DB as it requires reconnection.
		if c.Key == "DB_DSN" {
			continue
		}

		// If value empty, preserve .env settings.
		if c.Value == "" {
			continue
		}

		switch c.Key {
		case "BOT_TOKEN":
			w.globalConfig.BotToken = c.Value
		case "ADMIN_USERNAME":
			w.globalConfig.AdminUsername = c.Value
		case "SPOTIFY_CLIENT_ID":
			w.globalConfig.SpotifyClientID = c.Value
		case "SPOTIFY_CLIENT_SECRET":
			w.globalConfig.SpotifyClientSecret = c.Value
		case "PLAYLIST_URL":
			w.globalConfig.PlaylistURL = c.Value
		case "HEALTH_PORT":
			w.globalConfig.HealthPort = c.Value
		case "LOG_LEVEL":
			w.globalConfig.LogLevel = c.Value
		case "SCRAPER_REQUEST_DELAY":
			if d, err := time.ParseDuration(c.Value); err == nil {
				w.globalConfig.ScraperDelay = d
			}
		}
	}
	return nil
}

// ApplyConfigChanges reacts to specific configuration key changes by logging their application.
func (w *ConfigWatcher) ApplyConfigChanges(configs []model.Config) error {
	for _, config := range configs {
		switch config.Key {
		case "SCRAPER_DELAY":
			w.logger.Info("Applying scraper delay change", zap.String("value", config.Value))
		case "SCRAPER_TIMEOUT":
			w.logger.Info("Applying scraper timeout change", zap.String("value", config.Value))
		case "LOG_LEVEL":
			w.logger.Info("Applying log level change", zap.String("value", config.Value))
		default:
			w.logger.Debug("No specific handler for config key", zap.String("key", config.Key))
		}
	}
	return nil
}
