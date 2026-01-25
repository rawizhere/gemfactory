// Package service implements the core business logic and domain services.
package service

import (
	"context"
	"gemfactory/internal/config"
	"gemfactory/internal/scraper"
	"gemfactory/internal/spotify"
	"gemfactory/internal/storage"

	"github.com/uptrace/bun"
	"go.uber.org/zap"
)

// Services embeds all domain services used by the application.
type Services struct {
	Artist        *ArtistService
	Release       *ReleaseService
	Homework      *HomeworkService
	Playlist      *PlaylistService
	Config        *ConfigService
	ConfigWatcher *ConfigWatcher
}

// NewServices initializes and connects all application-layer services.
func NewServices(db *storage.Postgres, cfg *config.Config, logger *zap.Logger) *Services {
	configService := NewConfigService(db.GetDB(), logger)
	configLoader := NewConfigLoader(configService, logger)
	configLoader.LoadConfigFromDB(context.Background(), cfg)

	spotifyClient := NewSpotifyClient(cfg, logger)
	scraperClient := NewScraperClient(cfg, logger)
	playlistService := NewPlaylistServiceWithClient(db.GetDB(), spotifyClient, cfg.PlaylistURL, logger)

	coreServices := NewCoreServices(db, logger)
	coreServices.Release = NewReleaseService(db.GetDB(), scraperClient, logger)
	coreServices.Homework = NewHomeworkService(db.GetDB(), playlistService, cfg.HomeworkResetTime, logger)

	configWatcher := NewConfigWatcher(configService, cfg, logger)
	if scraperClient != nil {
		if configurable, ok := scraperClient.(Configurable); ok {
			configWatcher.Subscribe(configurable)
		}
	}

	return &Services{
		Artist:        coreServices.Artist,
		Release:       coreServices.Release,
		Homework:      coreServices.Homework,
		Playlist:      playlistService,
		Config:        configService,
		ConfigWatcher: configWatcher,
	}
}

// NewConfigLoader initializes a loader to synchronize database configuration with the application state.
func NewConfigLoader(configService *ConfigService, logger *zap.Logger) *config.ConfigLoader {
	return config.NewConfigLoader(configService, logger)
}

// NewSpotifyClient initializes the Spotify API client using provided credentials.
func NewSpotifyClient(cfg *config.Config, logger *zap.Logger) *spotify.Client {
	if cfg.SpotifyClientID == "" || cfg.SpotifyClientSecret == "" {
		logger.Warn("Spotify credentials not provided, Spotify client will not be created")
		return nil
	}

	client, err := spotify.NewClient(cfg.SpotifyClientID, cfg.SpotifyClientSecret, logger)
	if err != nil {
		logger.Error("Failed to create Spotify client", zap.Error(err))
		return nil
	}

	return client
}

// NewScraperClient initializes the K-Profiles web scraper with standard resilience settings.
func NewScraperClient(cfg *config.Config, logger *zap.Logger) scraper.Fetcher {
	scraperConfig := scraper.Config{
		HTTPClientConfig: scraper.HTTPClientConfig{
			MaxIdleConns:          config.DefaultMaxIdleConns,
			MaxIdleConnsPerHost:   config.DefaultMaxIdleConnsPerHost,
			IdleConnTimeout:       config.DefaultIdleConnTimeout,
			TLSHandshakeTimeout:   config.DefaultTLSHandshakeTimeout,
			ResponseHeaderTimeout: config.DefaultResponseHeaderTimeout,
			DisableKeepAlives:     config.DefaultDisableKeepAlives,
		},
		RetryConfig: scraper.RetryConfig{
			MaxRetries:        config.DefaultMaxRetries,
			InitialDelay:      config.DefaultInitialDelay,
			MaxDelay:          config.DefaultMaxDelay,
			BackoffMultiplier: config.DefaultBackoffMultiplier,
		},
		RequestDelay: cfg.ScraperDelay,
		UserAgent:    config.DefaultScraperUserAgent,
	}
	return scraper.NewFetcher(scraperConfig, logger)
}

// NewPlaylistServiceWithClient initializes the playlist service, requiring a valid Spotify client.
func NewPlaylistServiceWithClient(db *bun.DB, spotifyClient *spotify.Client, playlistURL string, logger *zap.Logger) *PlaylistService {
	if spotifyClient == nil {
		logger.Warn("Spotify client not available, playlist service will not be created")
		return nil
	}
	return NewPlaylistService(db, *spotifyClient, playlistURL, logger)
}

// CoreServices groups the primary domain services for simplified initialization.
type CoreServices struct {
	Artist   *ArtistService
	Release  *ReleaseService
	Homework *HomeworkService
}

// NewCoreServices initializes the core set of domain-specific business logic services.
func NewCoreServices(db *storage.Postgres, logger *zap.Logger) *CoreServices {
	return &CoreServices{
		Artist: NewArtistService(db.GetDB(), logger),
	}
}
