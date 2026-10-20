package service

import (
	"context"
	"fmt"
	"gemfactory/internal/model"

	"go.uber.org/zap"
)

type ConfigService struct {
	repo   model.ConfigRepository
	logger *zap.Logger
}

func NewConfigService(repo model.ConfigRepository, logger *zap.Logger) *ConfigService {
	return &ConfigService{repo: repo, logger: logger}
}

func (s *ConfigService) Update(ctx context.Context, key, value string) error {
	err := s.repo.Set(ctx, key, value)
	if err != nil {
		return fmt.Errorf("failed to update config %s: %w", key, err)
	}

	s.logger.Info("Config updated", zap.String("key", key), zap.String("value", value))
	return nil
}

func (s *ConfigService) Get(ctx context.Context, key string) (string, error) {
	c, err := s.repo.Get(ctx, key)
	if err != nil {
		return "", fmt.Errorf("failed to get config %s: %w", key, err)
	}
	if c == nil {
		return "", fmt.Errorf("config %s not found", key)
	}
	return c.Value, nil
}
