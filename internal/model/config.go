// Package model defines the data structures for application state and persistence.
package model

import (
	"context"
	"time"

	"github.com/uptrace/bun"
)

// Config represents a key-value pair for application configuration.
type Config struct {
	bun.BaseModel `bun:"table:gemfactory.config"`

	ID          int       `bun:"id,pk,autoincrement" json:"id"`
	Key         string    `bun:"key,unique,notnull" json:"key"`
	Value       string    `bun:"value,notnull" json:"value"`
	Description string    `bun:"description" json:"description"`
	CreatedAt   time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt   time.Time `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`
}

// ConfigRepository defines the interface for configuration operations.
type ConfigRepository interface {
	Get(ctx context.Context, key string) (*Config, error)
	GetAll(ctx context.Context) ([]Config, error)
	Set(ctx context.Context, key, value string) error
	Delete(ctx context.Context, key string) error
	Reset(ctx context.Context) error
	GetDefaultConfig() map[string]string
}
