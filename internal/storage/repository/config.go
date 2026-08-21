// Package repository contains database implementation.
package repository

import (
	"context"
	"fmt"
	"gemfactory/internal/model"

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

	err := r.db.NewSelect().
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

	err := r.db.NewSelect().
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
	config := &model.Config{
		Key:   key,
		Value: value,
	}

	_, err := r.db.NewInsert().
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
	_, err := r.db.NewDelete().
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
	_, err := r.db.NewDelete().
		Model((*model.Config)(nil)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete config: %w", err)
	}

	configs := []model.Config{
		{Key: "RATE_LIMIT_REQUESTS", Value: "10"},
		{Key: "RATE_LIMIT_WINDOW", Value: "60"},
		{Key: "SCRAPER_DELAY", Value: "2s"},
		{Key: "LOG_LEVEL", Value: "info"},
		{Key: "BOT_TOKEN", Value: ""},
		{Key: "ADMIN_USERNAME", Value: ""},
		{Key: "DB_DSN", Value: ""},
		{Key: "HEALTH_PORT", Value: "8080"},
	}

	_, err = r.db.NewInsert().
		Model(&configs).
		On("CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()").
		Exec(ctx)

	if err != nil {
		return fmt.Errorf("failed to insert default configs: %w", err)
	}

	return nil
}
