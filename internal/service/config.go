// Package service implements the core business logic and domain services.
package service

import (
	"context"
	"fmt"
	"gemfactory/internal/model"
	"gemfactory/internal/storage/repository"
	"strings"

	"github.com/uptrace/bun"
	"go.uber.org/zap"
)

// ConfigService provides methods for managing and retrieving application configuration settings.
type ConfigService struct {
	repo   model.ConfigRepository
	logger *zap.Logger
}

func NewConfigService(db *bun.DB, logger *zap.Logger) *ConfigService {
	return &ConfigService{
		repo:   repository.NewConfigRepository(db, logger),
		logger: logger,
	}
}

// Update modifies a configuration setting by key.
func (s *ConfigService) Update(ctx context.Context, key, value string) error {
	err := s.repo.Set(ctx, key, value)
	if err != nil {
		return fmt.Errorf("failed to update config %s: %w", key, err)
	}

	s.logger.Info("Config updated", zap.String("key", key), zap.String("value", value))
	return nil
}

// Get retrieves a configuration setting by key.
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

// GetAll returns a formatted HTML representation of all non-sensitive configuration settings.
func (s *ConfigService) GetAll(ctx context.Context) (string, error) {
	configs, err := s.repo.GetAll(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get all configs: %w", err)
	}

	sensitiveKeys := map[string]bool{
		"BOT_TOKEN": true,
	}

	var result strings.Builder
	result.WriteString("📋 Current Configuration:\n\n")

	for _, c := range configs {
		var value string
		if sensitiveKeys[c.Key] {
			value = "🔒 [HIDDEN FOR SECURITY - CHECK ENVIRONMENT]"
		} else {
			value = c.Value
		}

		fmt.Fprintf(&result, "🔧 <b>%s</b>: %s\n", c.Key, value)
		if c.Description != "" {
			fmt.Fprintf(&result, "   📝 %s\n", c.Description)
		}
		result.WriteString("\n")
	}

	return result.String(), nil
}

// GetAllRaw retrieves all configuration model objects.
func (s *ConfigService) GetAllRaw(ctx context.Context) ([]model.Config, error) {
	return s.repo.GetAll(ctx)
}

// Reset restores all configuration settings to their default values.
func (s *ConfigService) Reset(ctx context.Context) error {
	err := s.repo.Reset(ctx)
	if err != nil {
		return fmt.Errorf("failed to reset config: %w", err)
	}

	s.logger.Info("Config reset to default values")
	return nil
}
