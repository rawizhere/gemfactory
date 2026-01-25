// Package config provides utilities for dynamic configuration management.
package config

import (
	"context"

	"go.uber.org/zap"
)

// ConfigLoader handles configuration overrides from the database.
type ConfigLoader struct {
	configService ConfigServiceInterface
	logger        *zap.Logger
}

// ConfigServiceInterface defines the interaction with persistent configuration.
type ConfigServiceInterface interface {
	Get(ctx context.Context, key string) (string, error)
}

// NewConfigLoader initializes a configuration loader.
func NewConfigLoader(configService ConfigServiceInterface, logger *zap.Logger) *ConfigLoader {
	return &ConfigLoader{
		configService: configService,
		logger:        logger,
	}
}

// LoadConfigValue retrieves a setting with environment taking precedence over the database.
func (cl *ConfigLoader) LoadConfigValue(ctx context.Context, envValue, configKey string) string {
	if envValue == "" {
		if dbValue, err := cl.configService.Get(ctx, configKey); err == nil && dbValue != "" {
			cl.logger.Info("Loaded "+configKey+" from database", zap.String("value", dbValue))
			return dbValue
		} else {
			cl.logger.Debug("Failed to load "+configKey+" from database", zap.Error(err))
			return ""
		}
	} else {
		cl.logger.Info("Using "+configKey+" from environment variables", zap.String("value", envValue))
		return envValue
	}
}

// LoadConfigValueWithSetter retrieves a setting and applies it using the provided setter.
func (cl *ConfigLoader) LoadConfigValueWithSetter(ctx context.Context, envValue, configKey string, setter func(string)) string {
	value := cl.LoadConfigValue(ctx, envValue, configKey)
	if value != "" {
		setter(value)
	}
	return value
}

// LoadConfigFromDB overrides configuration values with data from the database.
func (cl *ConfigLoader) LoadConfigFromDB(ctx context.Context, cfg *Config) {
	// ADMIN_USERNAME.
	cl.LoadConfigValueWithSetter(ctx, cfg.AdminUsername, "ADMIN_USERNAME", func(value string) {
		cfg.AdminUsername = value
	})

	// BOT_TOKEN.
	cl.LoadConfigValueWithSetter(ctx, cfg.BotToken, "BOT_TOKEN", func(value string) {
		cfg.BotToken = value
	})

	// SPOTIFY_CLIENT_ID.
	cl.LoadConfigValueWithSetter(ctx, cfg.SpotifyClientID, "SPOTIFY_CLIENT_ID", func(value string) {
		cfg.SpotifyClientID = value
	})

	// SPOTIFY_CLIENT_SECRET.
	cl.LoadConfigValueWithSetter(ctx, cfg.SpotifyClientSecret, "SPOTIFY_CLIENT_SECRET", func(value string) {
		cfg.SpotifyClientSecret = value
	})

	// PLAYLIST_URL.
	cl.LoadConfigValueWithSetter(ctx, cfg.PlaylistURL, "PLAYLIST_URL", func(value string) {
		cfg.PlaylistURL = value
	})
}
