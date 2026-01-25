package scraper

import (
	"context"
	"gemfactory/internal/model"
	"time"
)

// Fetcher defines the contract for retrieving and parsing music release information.
type Fetcher interface {
	FetchMonthlyLinks(ctx context.Context, months []string, year string) ([]string, error)
	ParseKProfilesMonthlyPage(ctx context.Context, url, month, year string) ([]Release, error)
	ApplyConfig(ctx context.Context, configs []model.Config) error
}

// Config represents scraper configuration.
type Config struct {
	HTTPClientConfig HTTPClientConfig
	RetryConfig      RetryConfig
	RequestDelay     time.Duration
	UserAgent        string
}

// HTTPClientConfig contains settings for the internal HTTP transport layer.
type HTTPClientConfig struct {
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
	IdleConnTimeout       time.Duration
	TLSHandshakeTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
	DisableKeepAlives     bool
}

// RetryConfig represents retry mechanism configuration.
type RetryConfig struct {
	MaxRetries        int
	InitialDelay      time.Duration
	MaxDelay          time.Duration
	BackoffMultiplier float64
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
