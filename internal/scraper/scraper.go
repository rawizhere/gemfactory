package scraper

import (
	"go.uber.org/zap"
)

// Fetcher parses release calendars from kpopofficial.com.
type Fetcher struct {
	logger     *zap.Logger
	httpClient *HTTPClient
}

func NewFetcher(userAgent string, logger *zap.Logger) *Fetcher {
	return &Fetcher{
		logger:     logger,
		httpClient: NewHTTPClient(userAgent, logger),
	}
}
