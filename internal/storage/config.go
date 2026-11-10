package storage

import (
	"context"
	"fmt"
	"gemfactory/internal/model"

	"github.com/uptrace/bun"
)

type ConfigRepository struct {
	db *bun.DB
}

func NewConfigRepository(db *bun.DB) model.ConfigRepository {
	return &ConfigRepository{db: db}
}

func (r *ConfigRepository) Get(ctx context.Context, key string) (*model.Config, error) {
	return one(ctx, r.db.NewSelect().Where("key = ?", key), new(model.Config), "config")
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
