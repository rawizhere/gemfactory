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
	config, err := s.repo.Get(ctx, key)
	if err != nil {
		return "", fmt.Errorf("failed to get config %s: %w", key, err)
	}

	if config == nil {
		return "", fmt.Errorf("config %s not found", key)
	}

	return config.Value, nil
}

// GetConfigValue is an alias for Get.
func (s *ConfigService) GetConfigValue(ctx context.Context, key string) (string, error) {
	return s.Get(ctx, key)
}

// SetConfigValue is an alias for Update.
func (s *ConfigService) SetConfigValue(ctx context.Context, key, value string) error {
	return s.Update(ctx, key, value)
}

// GetAllConfig retrieves all configuration settings as a map.
func (s *ConfigService) GetAllConfig(ctx context.Context) (map[string]string, error) {
	configs, err := s.repo.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get all configs: %w", err)
	}

	result := make(map[string]string)
	for _, config := range configs {
		result[config.Key] = config.Value
	}

	return result, nil
}

// GetAll returns a formatted HTML representation of all non-sensitive configuration settings.
func (s *ConfigService) GetAll(ctx context.Context) (string, error) {
	configs, err := s.repo.GetAll(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get all configs: %w", err)
	}

	sensitiveKeys := map[string]bool{
		"BOT_TOKEN":             true,
		"SPOTIFY_CLIENT_ID":     true,
		"SPOTIFY_CLIENT_SECRET": true,
	}

	var result strings.Builder
	result.WriteString("📋 Current Configuration:\n\n")

	for _, config := range configs {
		var value string
		if sensitiveKeys[config.Key] {
			value = "🔒 [HIDDEN FOR SECURITY - CHECK ENVIRONMENT]"
		} else {
			value = config.Value
		}

		result.WriteString(fmt.Sprintf("🔧 <b>%s</b>: %s\n", config.Key, value))
		if config.Description != "" {
			result.WriteString(fmt.Sprintf("   📝 %s\n", config.Description))
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

// GetInt retrieves a configuration value parsed as an integer.
func (s *ConfigService) GetInt(ctx context.Context, key string) (int, error) {
	value, err := s.Get(ctx, key)
	if err != nil {
		return 0, err
	}

	var intValue int
	_, err = fmt.Sscanf(value, "%d", &intValue)
	if err != nil {
		return 0, fmt.Errorf("failed to parse config %s as int: %w", key, err)
	}

	return intValue, nil
}

// GetBool retrieves a configuration value parsed as a boolean.
func (s *ConfigService) GetBool(ctx context.Context, key string) (bool, error) {
	value, err := s.Get(ctx, key)
	if err != nil {
		return false, err
	}

	return strings.ToLower(value) == "true", nil
}

// GetFloat retrieves a configuration value parsed as a floating-point number.
func (s *ConfigService) GetFloat(ctx context.Context, key string) (float64, error) {
	value, err := s.Get(ctx, key)
	if err != nil {
		return 0, err
	}

	var floatValue float64
	_, err = fmt.Sscanf(value, "%f", &floatValue)
	if err != nil {
		return 0, fmt.Errorf("failed to parse config %s as float: %w", key, err)
	}

	return floatValue, nil
}

// SetInt updates a configuration setting with an integer value.
func (s *ConfigService) SetInt(ctx context.Context, key string, value int) error {
	return s.Update(ctx, key, fmt.Sprintf("%d", value))
}

// SetBool updates a configuration setting with a boolean value.
func (s *ConfigService) SetBool(ctx context.Context, key string, value bool) error {
	return s.Update(ctx, key, fmt.Sprintf("%t", value))
}

// SetFloat updates a configuration setting with a floating-point value.
func (s *ConfigService) SetFloat(ctx context.Context, key string, value float64) error {
	return s.Update(ctx, key, fmt.Sprintf("%f", value))
}
