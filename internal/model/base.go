// Package model defines core domain structures and persistence interfaces.
package model

import (
	"context"
	"time"

	"github.com/uptrace/bun"
)

// BaseModel provides common fields for database entities.
type BaseModel struct {
	bun.BaseModel

	ID        int       `bun:"id,pk,autoincrement" json:"id"`
	CreatedAt time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`
}

// TimestampedModel includes creation and update tracking.
type TimestampedModel struct {
	CreatedAt time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`
}

// Repository defines standard CRUD operations.
type Repository[T any] interface {
	GetByID(ctx context.Context, id int) (*T, error)
	Create(ctx context.Context, entity *T) error
	Update(ctx context.Context, entity *T) error
	Delete(ctx context.Context, id int) error
	GetAll(ctx context.Context) ([]T, error)
}
