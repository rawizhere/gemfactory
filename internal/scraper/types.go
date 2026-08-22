package scraper

import (
	"context"
	"iter"
	"time"
)

type Fetcher interface {
	ParseMonth(ctx context.Context, month, year string) iter.Seq2[Release, error]
	ParseYear(ctx context.Context, year string) iter.Seq2[Release, error]
}

type Config struct {
	RequestDelay time.Duration
	UserAgent    string
}

type Release struct {
	Date       time.Time
	Artist     string
	Title      string
	AlbumName  string
	TitleTrack string
	MV         string
	Spotify    string
	SourceURL  string
}
