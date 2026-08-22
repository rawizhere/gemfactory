package service

import (
	"context"
	"gemfactory/internal/config"
	"gemfactory/internal/scraper"
	"gemfactory/internal/storage"

	"go.uber.org/zap"
)

type Services struct {
	Artist  *ArtistService
	Release *ReleaseService
	Config  *ConfigService
}

func NewServices(db *storage.Postgres, cfg *config.Config, logger *zap.Logger) *Services {
	configService := NewConfigService(db.GetDB(), logger)
	config.OverrideFromDB(context.Background(), cfg, configService.Get, logger)

	scraperClient := scraper.NewFetcher(scraper.Config{
		RequestDelay: cfg.ScraperDelay,
		UserAgent:    config.DefaultScraperUserAgent,
	}, logger)

	return &Services{
		Artist:  NewArtistService(db.GetDB(), logger),
		Release: NewReleaseService(db.GetDB(), scraperClient, logger),
		Config:  configService,
	}
}
