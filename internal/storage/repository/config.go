// Package repository contains database implementation.
package repository

import (
	"context"
	"fmt"
	"gemfactory/internal/model"
	"strings"

	"github.com/uptrace/bun"
	"go.uber.org/zap"
)

// ConfigRepository manages persistent storage for application configuration settings.
type ConfigRepository struct {
	db     *bun.DB
	logger *zap.Logger
}

// NewConfigRepository initializes a new ConfigRepository.
func NewConfigRepository(db *bun.DB, logger *zap.Logger) model.ConfigRepository {
	return &ConfigRepository{
		db:     db,
		logger: logger,
	}
}

// Get retrieves a configuration entry by its unique key.
func (r *ConfigRepository) Get(ctx context.Context, key string) (*model.Config, error) {
	config := new(model.Config)

	// Set search_path for this request
	_, err := r.db.ExecContext(ctx, "SET search_path TO gemfactory, public")
	if err != nil {
		r.logger.Warn("Failed to set search_path", zap.Error(err))
	}

	err = r.db.NewSelect().
		Model(config).
		Where("key = ?", key).
		Scan(ctx)

	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to scan config: %w", err)
	}

	return config, nil
}

// GetAll retrieves all configuration settings, ordered by key.
func (r *ConfigRepository) GetAll(ctx context.Context) ([]model.Config, error) {
	var configs []model.Config

	_, err := r.db.ExecContext(ctx, "SET search_path TO gemfactory, public")
	if err != nil {
		r.logger.Warn("Failed to set search_path", zap.Error(err))
	}

	err = r.db.NewSelect().
		Model(&configs).
		Order("key ASC").
		Scan(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to query config: %w", err)
	}

	return configs, nil
}

// Set inserts or updates a configuration setting.
func (r *ConfigRepository) Set(ctx context.Context, key, value string) error {
	_, err := r.db.ExecContext(ctx, "SET search_path TO gemfactory, public")
	if err != nil {
		r.logger.Warn("Failed to set search_path", zap.Error(err))
	}

	config := &model.Config{
		Key:   key,
		Value: value,
	}

	_, err = r.db.NewInsert().
		Model(config).
		On("CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()").
		Exec(ctx)

	if err != nil {
		return fmt.Errorf("failed to set config: %w", err)
	}

	return nil
}

// Delete removes a configuration setting by key.
func (r *ConfigRepository) Delete(ctx context.Context, key string) error {
	_, err := r.db.ExecContext(ctx, "SET search_path TO gemfactory, public")
	if err != nil {
		r.logger.Warn("Failed to set search_path", zap.Error(err))
	}

	_, err = r.db.NewDelete().
		Model((*model.Config)(nil)).
		Where("key = ?", key).
		Exec(ctx)

	if err != nil {
		return fmt.Errorf("failed to delete config: %w", err)
	}

	return nil
}

// Reset clears all configuration settings and restores them to default values.
func (r *ConfigRepository) Reset(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, "SET search_path TO gemfactory, public")
	if err != nil {
		r.logger.Warn("Failed to set search_path", zap.Error(err))
	}

	_, err = r.db.NewDelete().
		Model((*model.Config)(nil)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete config: %w", err)
	}

	defaultConfig := r.GetDefaultConfig()
	for key, value := range defaultConfig {
		err := r.Set(ctx, key, value)
		if err != nil {
			return fmt.Errorf("failed to set default config %s: %w", key, err)
		}
	}

	return nil
}

// GetDefaultConfig returns a map containing the system's baseline configuration settings.
func (r *ConfigRepository) GetDefaultConfig() map[string]string {
	return map[string]string{
		"RATE_LIMIT_REQUESTS":   "10",
		"RATE_LIMIT_WINDOW":     "60",
		"SCRAPER_DELAY":         "1",
		"SCRAPER_TIMEOUT":       "30",
		"LOG_LEVEL":             "info",
		"BOT_TOKEN":             "",
		"ADMIN_USERNAME":        "",
		"SPOTIFY_CLIENT_ID":     "",
		"SPOTIFY_CLIENT_SECRET": "",
		"PLAYLIST_URL":          "",
		"DB_DSN":                "",
		"HEALTH_PORT":           "8080",
	}
}

// GetAllAsString returns a human-readable list of all current configuration settings.
func (r *ConfigRepository) GetAllAsString(ctx context.Context) (string, error) {
	configs, err := r.GetAll(ctx)
	if err != nil {
		return "", err
	}

	var result strings.Builder
	result.WriteString("📋 Current Configuration:\n\n")

	for _, config := range configs {
		result.WriteString(fmt.Sprintf("🔧 %s: %s\n", config.Key, config.Value))
	}

	return result.String(), nil
}
