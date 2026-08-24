package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"gemfactory/internal/model"

	"github.com/uptrace/bun"
	"go.uber.org/zap"
)

type ConfigRepository struct {
	db     *bun.DB
	logger *zap.Logger
}

func NewConfigRepository(db *bun.DB, logger *zap.Logger) model.ConfigRepository {
	return &ConfigRepository{
		db:     db,
		logger: logger,
	}
}

func (r *ConfigRepository) Get(ctx context.Context, key string) (*model.Config, error) {
	config := new(model.Config)

	err := r.db.NewSelect().
		Model(config).
		Where("key = ?", key).
		Scan(ctx)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to scan config: %w", err)
	}

	return config, nil
}

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

func (r *ConfigRepository) Reset(ctx context.Context) error {
	// Clear everything: managed settings are re-created on demand when saved,
	// and startup-only values live in env, not here.
	_, err := r.db.NewDelete().
		Model((*model.Config)(nil)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete config: %w", err)
	}
	return nil
}
