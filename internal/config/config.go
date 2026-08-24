// Package config manages configuration loading, defaults, and validation.
package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

const (
	DefaultScraperUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

// Config holds application settings loaded from the environment and database.
type Config struct {
	DatabaseURL          string        `env:"DB_DSN"`
	BotToken             string        `env:"BOT_TOKEN"`
	AdminUsername        string        `env:"ADMIN_USERNAME"`
	HealthPort           string        `env:"HEALTH_PORT" envDefault:"8080"`
	HealthCheckEnabled   bool          `env:"HEALTH_CHECK_ENABLED" envDefault:"true"`
	WebPort              string        `env:"WEB_PORT" envDefault:"9090"`
	WebEnabled           bool          `env:"WEB_ENABLED" envDefault:"true"`
	LogLevel             string        `env:"LOG_LEVEL" envDefault:"info"`
	Timezone             string        `env:"TIMEZONE" envDefault:"Europe/Moscow"`
	AppDataDir           string        `env:"APP_DATA_DIR" envDefault:"./data"`
	DownloadConcurrency  int           `env:"DOWNLOAD_CONCURRENCY" envDefault:"4"`
	ScraperDelay         time.Duration `env:"SCRAPER_REQUEST_DELAY" envDefault:"2s"`
	ReleaseCheckInterval time.Duration `env:"RELEASE_CHECK_INTERVAL" envDefault:"24h"`
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("config parsing failed: %w", err)
	}
	if cfg.DownloadConcurrency <= 0 {
		cfg.DownloadConcurrency = 4
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

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
