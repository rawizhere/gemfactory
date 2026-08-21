package scraper

import (
	"context"
	"fmt"

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

// FetchMonthlyLinks generates the set of URLs targeting K-pop comeback schedules.
func (f *fetcherImpl) FetchMonthlyLinks(ctx context.Context, months []string, year string) ([]string, error) {
	links := make([]string, 0, len(months))

	for _, month := range months {
		url := fmt.Sprintf("https://kpopofficial.com/kpop-comeback-schedule-%s-%s/", month, year)
		links = append(links, url)
		f.logger.Info("Generated monthly link", zap.String("url", url))
	}

	return links, nil
}
