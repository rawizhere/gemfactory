package scraper

import (
	"context"
	"fmt"
	"gemfactory/internal/model"
	"strconv"
	"time"

	"go.uber.org/zap"
)

type fetcherImpl struct {
	config     Config
	logger     *zap.Logger
	httpClient *HTTPClient
}

func NewFetcher(config Config, logger *zap.Logger) Fetcher {
	httpClient := NewHTTPClient(config.HTTPClientConfig, config.UserAgent, logger)

	return &fetcherImpl{
		config:     config,
		logger:     logger,
		httpClient: httpClient,
	}
}

// ApplyConfig dynamically updates scraper-specific settings from the database.
func (f *fetcherImpl) ApplyConfig(ctx context.Context, configs []model.Config) error {
	for _, c := range configs {
		if c.Key == "SCRAPER_DELAY" {
			delaySec, err := strconv.Atoi(c.Value)
			if err != nil {
				// Try parsing as float for values like "1.5"
				if delayFloat, err := strconv.ParseFloat(c.Value, 64); err == nil {
					f.config.RequestDelay = time.Duration(delayFloat * float64(time.Second))
				} else {
					return fmt.Errorf("invalid SCRAPER_DELAY: %v", c.Value)
				}
			} else {
				f.config.RequestDelay = time.Duration(delaySec) * time.Second
			}
			f.logger.Info("Scraper delay updated", zap.Duration("delay", f.config.RequestDelay))
		}
	}
	return nil
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
