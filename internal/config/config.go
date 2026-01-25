// Package config provides tools for loading, storing, and validating application-wide settings.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

const (
	// HTTP Client Defaults.
	DefaultMaxIdleConns          = 100
	DefaultMaxIdleConnsPerHost   = 10
	DefaultIdleConnTimeout       = 90 * time.Second
	DefaultTLSHandshakeTimeout   = 10 * time.Second
	DefaultResponseHeaderTimeout = 30 * time.Second
	DefaultDisableKeepAlives     = false

	// Retry Defaults.
	DefaultMaxRetries        = 3
	DefaultInitialDelay      = 1 * time.Second
	DefaultMaxDelay          = 30 * time.Second
	DefaultBackoffMultiplier = 2.0

	// Scraper Defaults.
	DefaultScraperUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

// Config defines the application's configuration structure.
type Config struct {
	// Database.
	DatabaseURL string

	// Telegram.
	BotToken      string
	AdminUsername string

	// Spotify.
	SpotifyClientID     string
	SpotifyClientSecret string
	PlaylistURL         string

	// Health.
	HealthPort         string
	HealthCheckEnabled bool

	// Logging.
	LogLevel string

	// Timezone.
	Timezone string

	// App Data Directory.
	AppDataDir string

	// Homework.
	HomeworkResetTime string

	// Scraper.
	ScraperDelay         time.Duration
	ReleaseCheckInterval time.Duration

	// Scraper Configuration (Advanced).
	Scraper ScraperConfig
}

// ScraperConfig holds advanced scraper settings.
type ScraperConfig struct {
	UserAgent    string
	MaxRetries   int
	InitialDelay time.Duration
	MaxDelay     time.Duration
}

// Load reads configuration from environment variables and .env files.
func Load() (*Config, error) {
	_ = godotenv.Load()

	config := &Config{
		DatabaseURL:          getEnv("DB_DSN", ""),
		BotToken:             getEnv("BOT_TOKEN", ""),
		AdminUsername:        getEnv("ADMIN_USERNAME", ""),
		SpotifyClientID:      getEnv("SPOTIFY_CLIENT_ID", ""),
		SpotifyClientSecret:  getEnv("SPOTIFY_CLIENT_SECRET", ""),
		PlaylistURL:          getEnv("PLAYLIST_URL", ""),
		HealthPort:           getEnv("HEALTH_PORT", "8080"),
		HealthCheckEnabled:   getEnvBool("HEALTH_CHECK_ENABLED", true),
		LogLevel:             getEnv("LOG_LEVEL", "info"),
		Timezone:             getEnv("TIMEZONE", "Europe/Moscow"),
		AppDataDir:           getEnv("APP_DATA_DIR", "./data"),
		HomeworkResetTime:    getEnv("HOMEWORK_RESET_TIME", "00:00"),
		ScraperDelay:         getEnvDuration("SCRAPER_REQUEST_DELAY", 2*time.Second),
		ReleaseCheckInterval: getEnvDuration("RELEASE_CHECK_INTERVAL", 24*time.Hour),
		Scraper: ScraperConfig{
			UserAgent:    getEnv("SCRAPER_USER_AGENT", DefaultScraperUserAgent),
			MaxRetries:   getEnvInt("SCRAPER_MAX_RETRIES", DefaultMaxRetries),
			InitialDelay: getEnvDuration("SCRAPER_INITIAL_DELAY", DefaultInitialDelay),
			MaxDelay:     getEnvDuration("SCRAPER_MAX_DELAY", DefaultMaxDelay),
		},
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return config, nil
}

func (c *Config) GetAppDataDir() string {
	return c.AppDataDir
}

func (c *Config) GetDatabaseURL() string {
	return c.DatabaseURL
}

func (c *Config) GetBotToken() string {
	return c.BotToken
}

func (c *Config) GetHealthPort() string {
	return c.HealthPort
}

func (c *Config) GetHealthCheckEnabled() bool {
	return c.HealthCheckEnabled
}

func (c *Config) GetSpotifyClientID() string {
	return c.SpotifyClientID
}

func (c *Config) GetSpotifyClientSecret() string {
	return c.SpotifyClientSecret
}

func (c *Config) GetAdminUsername() string {
	return c.AdminUsername
}

// Validate ensures all essential configuration parameters are present and within valid ranges.
func (c *Config) Validate() error {
	// Critical configuration requirements.
	if c.DatabaseURL == "" {
		return fmt.Errorf("DB_DSN is required")
	}

	if c.AdminUsername == "" {
		return fmt.Errorf("ADMIN_USERNAME is required")
	}

	if c.ScraperDelay < 100*time.Millisecond {
		return fmt.Errorf("SCRAPER_REQUEST_DELAY is too small (min 100ms)")
	}

	return nil
}

// getEnv retrieves an environment variable or returns a default value.
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvDuration parses an environment variable as time.Duration.
func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

// getEnvBool parses an environment variable as a boolean.
func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

// getEnvInt parses an environment variable as an integer.
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
