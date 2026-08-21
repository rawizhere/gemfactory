package config

import (
	"testing"
	"time"
)

func TestConfigValidate(t *testing.T) {
	cfg := &Config{
		DatabaseURL:   "postgres://localhost:5432/test",
		AdminUsername: "admin",
		Timezone:      "Europe/Moscow",
		ScraperDelay:  2 * time.Second,
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("Expected valid config, got error: %v", err)
	}

	invalidCfg := &Config{
		DatabaseURL: "",
	}
	if err := invalidCfg.Validate(); err == nil {
		t.Error("Expected error for empty DB_DSN, got nil")
	}

	invalidTzCfg := &Config{
		DatabaseURL:   "postgres://localhost:5432/test",
		AdminUsername: "admin",
		Timezone:      "Invalid/Zone_Name",
		ScraperDelay:  2 * time.Second,
	}
	if err := invalidTzCfg.Validate(); err == nil {
		t.Error("Expected error for invalid TIMEZONE, got nil")
	}
}
