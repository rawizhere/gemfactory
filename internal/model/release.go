// Package model defines core domain entities and persistence contracts.
package model

import (
	"context"
	"time"

	"github.com/uptrace/bun"
)

// Release represents a curated music release containing metadata and external links.
type Release struct {
	bun.BaseModel `bun:"table:gemfactory.releases"`

	ReleaseID  int       `bun:"release_id,pk,autoincrement" json:"release_id"`
	ArtistID   int       `bun:"artist_id,notnull" json:"artist_id"`
	Title      string    `bun:"title,notnull" json:"title"`
	TitleTrack string    `bun:"title_track" json:"title_track"`     // Title track name
	AlbumName  string    `bun:"album_name" json:"album_name"`       // Album name
	MV         string    `bun:"mv" json:"mv"`                       // MV link (YouTube)
	Spotify    string    `bun:"spotify" json:"spotify"`             // Spotify link
	SourceURL  string    `bun:"source_url" json:"source_url"`       // Stable identifier URL
	Date       time.Time `bun:"date,type:date,notnull" json:"date"` // Release date
	IsActive   bool      `bun:"is_active,notnull,default:true" json:"is_active"`
	CreatedAt  time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt  time.Time `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`

	// Relationships
	Artist *Artist `bun:"rel:belongs-to,join:artist_id=artist_id" json:"artist,omitempty"`
}

// ReleaseRepository defines the interface for release operations.
type ReleaseRepository interface {
	Repository[Release]
	GetByGender(ctx context.Context, gender Gender) ([]Release, error)
	GetByArtistID(ctx context.Context, artistID int) ([]Release, error)
	GetByArtist(ctx context.Context, artistName string) ([]Release, error)
	GetByDateRange(ctx context.Context, start, end time.Time) ([]Release, error)
	GetActive(ctx context.Context) ([]Release, error)
	GetWithRelations(ctx context.Context) ([]Release, error)
	GetByArtistAndTitle(ctx context.Context, artistID int, title string) (*Release, error)
	GetByArtistDateAndTrack(ctx context.Context, artistID int, date time.Time, titleTrack string) (*Release, error)
	GetByArtistDateAndYouTube(ctx context.Context, artistID int, date time.Time, youtubeURL string) (*Release, error)
	GetByArtistDateAndSource(ctx context.Context, artistID int, date time.Time, sourceURL string) (*Release, error)
	GetTotalCount(ctx context.Context) (int, error)
}

// ScrapedReleaseData represents release data from scraper.
type ScrapedReleaseData struct {
	Artist    string    `json:"artist"`
	Title     string    `json:"title"`
	Date      string    `json:"date"`
	Type      string    `json:"type"`
	Gender    string    `json:"gender"`
	ScrapedAt time.Time `json:"scraped_at"`
}
