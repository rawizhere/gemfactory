// Package model defines the domain entities and repository interfaces for the bot.
package model

import (
	"context"
	"time"

	"github.com/uptrace/bun"
)

// HomeworkTracking represents the audit trail and current status of assigned homework.
type HomeworkTracking struct {
	bun.BaseModel `bun:"table:gemfactory.homeworks"`

	ID          int        `bun:"id,pk,autoincrement" json:"id"`
	UserID      int64      `bun:"user_id,notnull" json:"user_id"`
	TrackID     string     `bun:"track_id,notnull" json:"track_id"`
	SpotifyID   string     `bun:"spotify_id,notnull" json:"spotify_id"`
	PlayCount   int        `bun:"play_count,notnull,default:1" json:"play_count"`
	IssuedAt    time.Time  `bun:"issued_at,notnull,default:current_timestamp" json:"issued_at"`
	CompletedAt *time.Time `bun:"completed_at" json:"completed_at"`
	IsCompleted bool       `bun:"is_completed,notnull,default:false" json:"is_completed"`
	CreatedAt   time.Time  `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt   time.Time  `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`
}

// HomeworkTrackingRepository defines the interface for homework tracking operations.
type HomeworkTrackingRepository interface {
	GetByUserID(ctx context.Context, userID int64) ([]HomeworkTracking, error)
	GetCompletedByUserID(ctx context.Context, userID int64) ([]HomeworkTracking, error)
	GetPendingByUserID(ctx context.Context, userID int64) ([]HomeworkTracking, error)
	GetAllPending(ctx context.Context) ([]HomeworkTracking, error)
	Create(ctx context.Context, tracking *HomeworkTracking) error
	Update(ctx context.Context, tracking *HomeworkTracking) error
	MarkCompleted(ctx context.Context, userID int64, trackID string, spotifyID string) error
	GetIssuedTrackIDs(ctx context.Context, userID int64, spotifyID string) ([]string, error)
	GetLastRequestTime(ctx context.Context, userID int64) (*time.Time, error)
	GetTotalAssignedCount(ctx context.Context) (int, error)
	GetUniqueUsersCount(ctx context.Context) (int, error)
}
