package service

import (
	"context"

	"gemfactory/internal/model"
	"gemfactory/internal/scraper"
	"gemfactory/internal/settings"

	"log/slog"
)

type Services struct {
	Artist   *ArtistService
	Release  *ReleaseService
	Settings *settings.Provider
	Scraper  *scraper.Fetcher
}

func NewServices(artists model.ArtistRepository, releases model.ReleaseRepository, settingsProvider *settings.Provider, logger *slog.Logger) *Services {
	scraperClient := scraper.NewFetcher(scraper.ParseUserAgents(settingsProvider.Value(context.Background(), settings.ScraperUserAgents)), logger)

	return &Services{
		Artist:   NewArtistService(artists),
		Release:  NewReleaseService(releases, artists, scraperClient, logger),
		Settings: settingsProvider,
		Scraper:  scraperClient,
	}
}
