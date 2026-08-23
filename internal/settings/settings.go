// Package settings resolves configuration values with the DB → env → default precedence.
package settings

import (
	"context"
	"os"
	"strconv"
	"strings"

	"gemfactory/internal/model"
)

// Provider reads configuration values from the config repository,
// falling back to environment variables and then to caller defaults.
type Provider struct {
	configs model.ConfigRepository
}

func New(configs model.ConfigRepository) Provider {
	return Provider{configs: configs}
}

// Value returns the stored value for key, or env value, or def when unset/blank.
func (p Provider) Value(ctx context.Context, key, def string) string {
	if p.configs != nil {
		if c, err := p.configs.Get(ctx, key); err == nil && c != nil && strings.TrimSpace(c.Value) != "" {
			return strings.TrimSpace(c.Value)
		}
	}
	if env := os.Getenv(key); strings.TrimSpace(env) != "" {
		return strings.TrimSpace(env)
	}
	return def
}

// Int returns the stored integer value for key, or def when unset or unparseable.
func (p Provider) Int(ctx context.Context, key string, def int) int {
	v, err := strconv.Atoi(p.Value(ctx, key, ""))
	if err != nil || v <= 0 {
		return def
	}
	return v
}

// Bool reports whether the stored value is truthy ("1", "true", "yes", "on").
func (p Provider) Bool(ctx context.Context, key string, def bool) bool {
	switch strings.ToLower(p.Value(ctx, key, "")) {
	case "":
		return def
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
