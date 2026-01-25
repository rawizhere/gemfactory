// Package model provides structures for playlist data.
package model

import (
	"context"
	"time"

	"github.com/uptrace/bun"
)

// PlaylistTracks models a track within a specific Spotify playlist.
type PlaylistTracks struct {
	bun.BaseModel `bun:"table:gemfactory.playlist_tracks"`

	ID        int       `bun:"id,pk,autoincrement" json:"id"`
	SpotifyID string    `bun:"spotify_id,notnull" json:"spotify_id"`
	TrackID   string    `bun:"track_id,notnull" json:"track_id"`
	Artist    string    `bun:"artist,notnull" json:"artist"`
	Title     string    `bun:"title,notnull" json:"title"`
	AddedAt   time.Time `bun:"added_at,notnull,default:current_timestamp" json:"added_at"`
	CreatedAt time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`
}

// PlaylistTracksRepository manages persistence for playlist tracks.
type PlaylistTracksRepository interface {
	GetBySpotifyID(ctx context.Context, spotifyID string) ([]PlaylistTracks, error)
	GetRandomTrack(ctx context.Context, spotifyID string, excludeTrackIDs []string) (*PlaylistTracks, error)
	Create(ctx context.Context, track *PlaylistTracks) error
	Update(ctx context.Context, track *PlaylistTracks) error
	Delete(ctx context.Context, id int) error
	DeleteBySpotifyID(ctx context.Context, spotifyID string) error
	GetAllBySpotifyID(ctx context.Context, spotifyID string) ([]PlaylistTracks, error)
}
