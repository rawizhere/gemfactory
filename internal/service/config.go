package service

import (
	"context"
	"fmt"
	"gemfactory/internal/model"
	"strings"

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

func (s *ConfigService) GetAll(ctx context.Context) (string, error) {
	configs, err := s.repo.GetAll(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get all configs: %w", err)
	}

	sensitiveKeys := map[string]bool{
		"BOT_TOKEN": true,
	}

	var result strings.Builder
	result.WriteString("Current Configuration:\n\n")

	for _, c := range configs {
		var value string
		if sensitiveKeys[c.Key] {
			value = "[HIDDEN FOR SECURITY - CHECK ENVIRONMENT]"
		} else {
			value = c.Value
		}

		fmt.Fprintf(&result, "<b>%s</b>: %s\n", c.Key, value)
		if c.Description != "" {
			fmt.Fprintf(&result, "   %s\n", c.Description)
		}
		result.WriteString("\n")
	}

	return result.String(), nil
}
