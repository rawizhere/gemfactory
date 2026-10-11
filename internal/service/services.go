package service

import (
	"context"
	"os"

	"gemfactory/internal/config"
	"gemfactory/internal/scraper"
	"gemfactory/internal/settings"
	"gemfactory/internal/storage"

	"go.uber.org/zap"
)

type Services struct {
	Artist   *ArtistService
	Release  *ReleaseService
	Config   *ConfigService
	Settings settings.Provider
	Scraper  *scraper.Fetcher
}

func NewServices(db *storage.Postgres, cfg *config.Config, logger *zap.Logger) *Services {
	configRepo := storage.NewConfigRepository(db.GetDB(), logger)
	configSvc := NewConfigService(configRepo, logger)
	scraperClient := scraper.NewFetcher(initialUserAgents(context.Background(), configSvc), logger)

	return &Services{
		Artist:   NewArtistService(db.GetDB(), logger),
		Release:  NewReleaseService(db.GetDB(), scraperClient, logger),
		Config:   configSvc,
		Settings: settings.New(configRepo),
		Scraper:  scraperClient,
	}
}

// initialUserAgents prefers DB config, then the env override, then compiled defaults.
func initialUserAgents(ctx context.Context, configSvc *ConfigService) []string {
	if v, err := configSvc.Get(ctx, "SCRAPER_USER_AGENTS"); err == nil {
		if uas := scraper.ParseUserAgents(v); len(uas) > 0 {
			return uas
		}
	}
	if uas := scraper.ParseUserAgents(os.Getenv("SCRAPER_USER_AGENTS")); len(uas) > 0 {
		return uas
	}
	return config.DefaultScraperUserAgents
}
