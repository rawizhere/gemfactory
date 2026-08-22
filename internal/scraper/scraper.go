package scraper

import (
	"go.uber.org/zap"
)

type fetcherImpl struct {
	config     Config
	logger     *zap.Logger
	httpClient *HTTPClient
}

// NewFetcher creates a new web Fetcher instance.
func NewFetcher(config Config, logger *zap.Logger) Fetcher {
	httpClient := NewHTTPClient(config.UserAgent, logger)

	return &fetcherImpl{
		config:     config,
		logger:     logger,
		httpClient: httpClient,
	}
}
