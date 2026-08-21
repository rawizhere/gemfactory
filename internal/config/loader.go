package config

import (
	"context"

	"go.uber.org/zap"
)

func OverrideFromDB(ctx context.Context, cfg *Config, get func(context.Context, string) (string, error), logger *zap.Logger) {
	if cfg.AdminUsername == "" {
		if val, err := get(ctx, "ADMIN_USERNAME"); err == nil && val != "" {
			logger.Info("Loaded ADMIN_USERNAME from database")
			cfg.AdminUsername = val
		}
	}

	if cfg.BotToken == "" {
		if val, err := get(ctx, "BOT_TOKEN"); err == nil && val != "" {
			logger.Info("Loaded BOT_TOKEN from database")
			cfg.BotToken = val
		}
	}
}
