package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConfigValidate(t *testing.T) {
	cfg := &Config{
		DatabaseURL:   "postgres://localhost:5432/test",
		AdminUsername: "admin",
		Timezone:      "Europe/Moscow",
		ScraperDelay:  2 * time.Second,
	}
	require.NoError(t, cfg.Validate(), "expected valid config")

	invalidCfg := &Config{
		DatabaseURL: "",
	}
	require.Error(t, invalidCfg.Validate(), "expected error for empty DB_DSN")

	invalidTzCfg := &Config{
		DatabaseURL:   "postgres://localhost:5432/test",
		AdminUsername: "admin",
		Timezone:      "Invalid/Zone_Name",
		ScraperDelay:  2 * time.Second,
	}
	require.Error(t, invalidTzCfg.Validate(), "expected error for invalid TIMEZONE")
}
