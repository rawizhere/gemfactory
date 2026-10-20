package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigValidate(t *testing.T) {
	cfg := &Config{
		DatabaseURL:   "postgres://localhost:5432/test",
		AdminUsername: "admin",
		Timezone:      "Europe/Moscow",
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
	}
	require.Error(t, invalidTzCfg.Validate(), "expected error for invalid TIMEZONE")
}
