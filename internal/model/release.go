package model

import (
	"context"
	"time"

	"github.com/uptrace/bun"
)

type Release struct {
	bun.BaseModel `bun:"table:gemfactory.releases"`

	ReleaseID     int          `bun:"release_id,pk,autoincrement" json:"release_id"`
	ArtistID      int          `bun:"artist_id,notnull" json:"artist_id"`
	DisplayArtist UniqueString `bun:"display_artist" json:"display_artist,omitempty"`
	Title         UniqueString `bun:"title,notnull" json:"title"`
	TitleTrack    UniqueString `bun:"title_track" json:"title_track"`
	AlbumName     UniqueString `bun:"album_name" json:"album_name"`
	MV            UniqueString `bun:"mv" json:"mv"`
	Spotify       UniqueString `bun:"spotify" json:"spotify"`
	SourceURL     UniqueString `bun:"source_url" json:"source_url"`
	Date          time.Time    `bun:"date,type:date,notnull" json:"date"`
	IsActive      bool         `bun:"is_active,notnull,default:true" json:"is_active"`
	CreatedAt     time.Time    `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt     time.Time    `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`

	Artist *Artist `bun:"rel:belongs-to,join:artist_id=artist_id" json:"artist,omitempty"`
}

type ReleaseRepository interface {
	GetByID(ctx context.Context, id int) (*Release, error)
	GetAll(ctx context.Context) ([]Release, error)
	GetByGender(ctx context.Context, gender Gender) ([]Release, error)
	GetByArtistID(ctx context.Context, artistID int) ([]Release, error)
	GetByArtist(ctx context.Context, artistName string) ([]Release, error)
	GetByDateRange(ctx context.Context, start, end time.Time) ([]Release, error)
	GetActive(ctx context.Context) ([]Release, error)
	GetByArtistAndTitle(ctx context.Context, artistID int, title string) (*Release, error)
	GetByArtistDateAndTrack(ctx context.Context, artistID int, date time.Time, titleTrack string) (*Release, error)
	GetByArtistDateAndSource(ctx context.Context, artistID int, date time.Time, sourceURL string) (*Release, error)
	GetTotalCount(ctx context.Context) (int, error)
	Create(ctx context.Context, release *Release) error
	Update(ctx context.Context, release *Release) error
	Delete(ctx context.Context, id int) error
	DeleteByIDs(ctx context.Context, ids []int) (int, error)
	Upsert(ctx context.Context, release *Release) error
}
