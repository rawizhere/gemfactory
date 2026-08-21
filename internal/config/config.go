// Package config manages configuration loading, defaults, and validation.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

const (
	DefaultScraperUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

// Config holds application settings loaded from the environment and database.
type Config struct {
	DatabaseURL          string
	BotToken             string
	AdminUsername        string
	HealthPort           string
	HealthCheckEnabled   bool
	LogLevel             string
	Timezone             string
	AppDataDir           string
	ScraperDelay         time.Duration
	ReleaseCheckInterval time.Duration
}

// Load reads environment variables from .env and system env, returning a validated Config.
func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		DatabaseURL:          getEnv("DB_DSN", ""),
		BotToken:             getEnv("BOT_TOKEN", ""),
		AdminUsername:        getEnv("ADMIN_USERNAME", ""),
		HealthPort:           getEnv("HEALTH_PORT", "8080"),
		HealthCheckEnabled:   getEnvBool("HEALTH_CHECK_ENABLED", true),
		LogLevel:             getEnv("LOG_LEVEL", "info"),
		Timezone:             getEnv("TIMEZONE", "Europe/Moscow"),
		AppDataDir:           getEnv("APP_DATA_DIR", "./data"),
		ScraperDelay:         getEnvDuration("SCRAPER_REQUEST_DELAY", 2*time.Second),
		ReleaseCheckInterval: getEnvDuration("RELEASE_CHECK_INTERVAL", 24*time.Hour),
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// Validate checks that all required configuration fields are set and within acceptable bounds.
func (c *Config) Validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("DB_DSN is required")
	}
	if c.AdminUsername == "" {
		return fmt.Errorf("ADMIN_USERNAME is required")
	}
	if c.ScraperDelay < 100*time.Millisecond {
		return fmt.Errorf("SCRAPER_REQUEST_DELAY is too small (min 100ms)")
	}
	if _, err := time.LoadLocation(c.Timezone); err != nil {
		return fmt.Errorf("invalid TIMEZONE %q: %w", c.Timezone, err)
	}
	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}
