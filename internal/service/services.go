package service

import (
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
	scraperClient := scraper.NewFetcher(config.DefaultScraperUserAgent, logger)

	return &Services{
		Artist:  NewArtistService(db.GetDB(), logger),
		Release: NewReleaseService(db.GetDB(), scraperClient, logger),
		Config:  NewConfigService(db.GetDB(), logger),
	}
}
