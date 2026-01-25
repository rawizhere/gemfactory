// Package service contains business logic.
package service

import (
	"context"
	"fmt"
	"gemfactory/internal/model"
	"gemfactory/internal/spotify"
	"gemfactory/internal/storage/repository"
	"sync"
	"time"

	"github.com/uptrace/bun"
	"go.uber.org/zap"
)

// PlaylistService handles synchronization and retrieval of Spotify playlist data.
type PlaylistService struct {
	playlistRepo  model.PlaylistTracksRepository
	configRepo    model.ConfigRepository
	spotifyClient spotify.Client
	playlistURL   string
	logger        *zap.Logger

	infoCache     *spotify.PlaylistInfo
	infoCacheTime time.Time
	mu            sync.RWMutex
}

func NewPlaylistService(db *bun.DB, spotifyClient spotify.Client, playlistURL string, logger *zap.Logger) *PlaylistService {
	return &PlaylistService{
		playlistRepo:  repository.NewPlaylistTracksRepository(db, logger),
		configRepo:    repository.NewConfigRepository(db, logger),
		spotifyClient: spotifyClient,
		playlistURL:   playlistURL,
		logger:        logger,
	}
}

// Reload synchronizes the local database with the current Spotify playlist state.
func (s *PlaylistService) Reload(ctx context.Context) error {
	playlistURL := s.playlistURL
	if playlistURL == "" {
		config, err := s.configRepo.Get(ctx, "PLAYLIST_URL")
		if err != nil {
			return fmt.Errorf("failed to get playlist URL from config: %w", err)
		}
		if config != nil {
			playlistURL = config.Value
		}
	}

	if playlistURL == "" {
		return fmt.Errorf("playlist URL not configured")
	}

	spotifyID, err := s.spotifyClient.ExtractID(playlistURL)
	s.logger.Info("Extracted Spotify ID in PlaylistService", zap.String("playlist_url", playlistURL), zap.String("spotify_id", spotifyID))
	if err != nil || spotifyID == "" {
		return fmt.Errorf("failed to extract Spotify ID from playlist URL: %w", err)
	}

	s.logger.Info("Starting playlist reload", zap.String("spotify_id", spotifyID))

	playlistInfo, err := s.spotifyClient.GetInfo(playlistURL)
	if err != nil {
		return fmt.Errorf("failed to get playlist info: %w", err)
	}

	s.logger.Info("Got playlist info",
		zap.String("name", playlistInfo.Name),
		zap.Int("track_count", playlistInfo.TrackCount))

	err = s.playlistRepo.DeleteBySpotifyID(ctx, spotifyID)
	if err != nil {
		return fmt.Errorf("failed to delete old tracks: %w", err)
	}

	s.logger.Info("Deleted old tracks")

	tracks, err := s.spotifyClient.GetTracks(playlistURL)
	if err != nil {
		return fmt.Errorf("failed to get playlist tracks: %w", err)
	}

	s.logger.Info("Got tracks from Spotify", zap.Int("count", len(tracks)))

	savedCount := 0
	for _, track := range tracks {
		playlistTrack := &model.PlaylistTracks{
			SpotifyID: spotifyID,
			TrackID:   track.ID,
			Artist:    track.Artist,
			Title:     track.Title,
			AddedAt:   time.Now(),
		}

		err = s.playlistRepo.Create(ctx, playlistTrack)
		if err != nil {
			s.logger.Error("Failed to save track",
				zap.String("track_id", track.ID),
				zap.String("artist", track.Artist),
				zap.String("title", track.Title),
				zap.Error(err))
			continue
		}

		savedCount++
	}

	s.logger.Info("Playlist reload completed",
		zap.String("spotify_id", spotifyID),
		zap.Int("saved_tracks", savedCount),
		zap.Int("total_tracks", len(tracks)))

	return nil
}

// Update is an alias for Reload.
func (s *PlaylistService) Update(ctx context.Context) error {
	return s.Reload(ctx)
}

// GetTracks retrieves the current list of tracks from the local database.
func (s *PlaylistService) GetTracks(ctx context.Context) ([]model.PlaylistTracks, error) {
	spotifyID, err := s.spotifyClient.ExtractID(s.playlistURL)
	if err != nil {
		return nil, fmt.Errorf("invalid playlist URL: %w", err)
	}

	tracks, err := s.playlistRepo.GetBySpotifyID(ctx, spotifyID)
	if err != nil {
		return nil, fmt.Errorf("failed to get playlist tracks: %w", err)
	}

	return tracks, nil
}

// GetInfo retrieves playlist metadata, using a short-lived cache to minimize API calls.
func (s *PlaylistService) GetInfo(ctx context.Context) (*spotify.PlaylistInfo, error) {
	s.mu.RLock()
	if s.infoCache != nil && time.Since(s.infoCacheTime) < 1*time.Hour {
		defer s.mu.RUnlock()
		return s.infoCache, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Double check after lock.
	if s.infoCache != nil && time.Since(s.infoCacheTime) < 1*time.Hour {
		return s.infoCache, nil
	}

	playlistURL := s.playlistURL
	if playlistURL == "" {
		config, err := s.configRepo.Get(ctx, "PLAYLIST_URL")
		if err != nil {
			return nil, fmt.Errorf("failed to get playlist URL from config: %w", err)
		}
		if config != nil {
			playlistURL = config.Value
		}
	}

	if playlistURL == "" {
		return nil, fmt.Errorf("playlist URL not configured")
	}

	// Get playlist info from Spotify.
	playlistInfo, err := s.spotifyClient.GetInfo(playlistURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get playlist info: %w", err)
	}

	// Update cache.
	s.infoCache = playlistInfo
	s.infoCacheTime = time.Now()

	return playlistInfo, nil
}
