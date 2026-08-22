package scraper

import (
	"context"
	"iter"
	"time"
)

// Fetcher defines the contract for retrieving and parsing music release information.
type Fetcher interface {
	ParseMonth(ctx context.Context, month, year string) iter.Seq2[Release, error]
	ParseYear(ctx context.Context, year string) iter.Seq2[Release, error]
}

// Config represents scraper configuration.
type Config struct {
	RequestDelay time.Duration
	UserAgent    string
}

// Release encapsulates the data for a single crawled music release.
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
