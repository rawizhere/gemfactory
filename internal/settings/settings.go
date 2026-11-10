// Package settings resolves configuration values with the DB → env → default precedence.
package settings

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"

	"gemfactory/internal/model"
)

// Provider serves config values from an in-memory copy of the config table,
// falling back to environment variables and then to the key's default.
// One Provider is shared by the app; writes through Set refresh the copy. A nil Provider reads env and defaults only.
type Provider struct {
	repo model.ConfigRepository
	mu   sync.Mutex
	rows []model.Config    // nil until loaded
	vals map[string]string // key -> trimmed value
}

func New(repo model.ConfigRepository) *Provider {
	return &Provider{repo: repo}
}

// load returns the cached values, reading the table on first use or after Invalidate.
// A failed read is not cached, so the next call retries.
func (p *Provider) load(ctx context.Context) ([]model.Config, map[string]string, error) {
	if p == nil || p.repo == nil {
		return nil, nil, nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.vals == nil {
		rows, err := p.repo.GetAll(ctx)
		if err != nil {
			return nil, nil, err
		}
		p.rows, p.vals = rows, make(map[string]string, len(rows))
		for _, c := range rows {
			p.vals[c.Key] = strings.TrimSpace(c.Value)
		}
	}
	return p.rows, p.vals, nil
}

// Invalidate drops the cached table; the next read reloads it.
func (p *Provider) Invalidate() {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.rows, p.vals = nil, nil
	p.mu.Unlock()
}

// Set stores a value in the config table and refreshes the cache.
func (p *Provider) Set(ctx context.Context, name, value string) error {
	err := p.repo.Set(ctx, name, value)
	p.Invalidate()
	return err
}

// All returns every stored config row, ordered as the repository returns them.
func (p *Provider) All(ctx context.Context) ([]model.Config, error) {
	rows, _, err := p.load(ctx)
	return rows, err
}

// Resolve returns the effective value of k and its source: "db", "env" or "default".
func (p *Provider) Resolve(ctx context.Context, k Key) (value, source string) {
	if _, vals, _ := p.load(ctx); vals[k.Name] != "" {
		return vals[k.Name], "db"
	}
	if env := strings.TrimSpace(os.Getenv(k.Name)); env != "" {
		return env, "env"
	}
	return k.Def, "default"
}

// Value returns the effective value of k.
func (p *Provider) Value(ctx context.Context, k Key) string {
	v, _ := p.Resolve(ctx, k)
	return v
}

// Int returns the effective value of k as a positive integer, falling back to the key's default.
func (p *Provider) Int(ctx context.Context, k Key) int {
	if v, err := strconv.Atoi(p.Value(ctx, k)); err == nil && v > 0 {
		return v
	}
	v, _ := strconv.Atoi(k.Def)
	return v
}

// Bool reports whether the effective value of k is truthy.
func (p *Provider) Bool(ctx context.Context, k Key) bool {
	return IsTruthy(p.Value(ctx, k))
}

// IsTruthy reports whether s is a truthy string ("1", "true", "yes", "on").
func IsTruthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
