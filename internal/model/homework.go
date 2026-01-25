// Package model defines core domain entities and persistence contracts.
package model

import (
	"context"
	"time"

	"github.com/uptrace/bun"
)

// Homework represents a track assignment given to a specific user.
type Homework struct {
	bun.BaseModel `bun:"table:gemfactory.homeworks"`

	HomeworkID int       `bun:"homework_id,pk,autoincrement" json:"homework_id"`
	UserID     int64     `bun:"user_id,notnull" json:"user_id"`
	TrackID    string    `bun:"track_id,notnull" json:"track_id"`
	Artist     string    `bun:"artist,notnull" json:"artist"`
	Title      string    `bun:"title,notnull" json:"title"`
	PlayCount  int       `bun:"play_count,notnull,default:1" json:"play_count"`
	Completed  bool      `bun:"completed,notnull,default:false" json:"completed"`
	CreatedAt  time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt  time.Time `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`
}

// HomeworkRepository. Defines the interface for homework operations.
type HomeworkRepository interface {
	GetByUserID(ctx context.Context, userID int64) ([]Homework, error)
	GetActiveByUserID(ctx context.Context, userID int64) (*Homework, error)
	Create(ctx context.Context, homework *Homework) error
	Update(ctx context.Context, homework *Homework) error
	Delete(ctx context.Context, id int) error
	MarkCompleted(ctx context.Context, id int) error
	GetRandomTrack(ctx context.Context) (*Homework, error)
	CanRequestHomework(ctx context.Context, userID int64) (bool, error)
	GetLastRequestTime(ctx context.Context, userID int64) (*time.Time, error)
}
