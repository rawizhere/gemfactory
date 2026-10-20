package service

import (
	"context"

	"gemfactory/internal/config"
	"gemfactory/internal/model"
	"gemfactory/internal/scraper"
	"gemfactory/internal/settings"
	"gemfactory/internal/storage"

	"go.uber.org/zap"
)

type Services struct {
	Artist   *ArtistService
	Release  *ReleaseService
	Configs  model.ConfigRepository
	Settings settings.Provider
	Scraper  *scraper.Fetcher
}

func NewServices(db *storage.Postgres, cfg *config.Config, logger *zap.Logger) *Services {
	configRepo := storage.NewConfigRepository(db.GetDB())
	settingsProvider := settings.New(configRepo)
	scraperClient := scraper.NewFetcher(initialUserAgents(context.Background(), settingsProvider), logger)

	return &Services{
		Artist:   NewArtistService(db.GetDB()),
		Release:  NewReleaseService(db.GetDB(), scraperClient, logger),
		Configs:  configRepo,
		Settings: settingsProvider,
		Scraper:  scraperClient,
	}
}

// initialUserAgents prefers DB config, then the env override, then compiled defaults.
func initialUserAgents(ctx context.Context, s settings.Provider) []string {
	if uas := scraper.ParseUserAgents(s.Value(ctx, "SCRAPER_USER_AGENTS", "")); len(uas) > 0 {
		return uas
	}
	return config.DefaultScraperUserAgents
}
