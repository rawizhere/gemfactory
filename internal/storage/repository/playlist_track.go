// Package repository provides database-specific implementations of domain repositories.
package repository

import (
	"context"
	"fmt"

	"gemfactory/internal/model"

	"github.com/uptrace/bun"
	"go.uber.org/zap"
)

// PlaylistTracksRepository manages persistent storage for tracks within Spotify playlists.
type PlaylistTracksRepository struct {
	db     *bun.DB
	logger *zap.Logger
}

// NewPlaylistTracksRepository initializes a new PlaylistTracksRepository.
func NewPlaylistTracksRepository(db *bun.DB, logger *zap.Logger) *PlaylistTracksRepository {
	return &PlaylistTracksRepository{
		db:     db,
		logger: logger,
	}
}

// GetBySpotifyID retrieves all tracks associated with a specific Spotify playlist ID.
func (r *PlaylistTracksRepository) GetBySpotifyID(ctx context.Context, spotifyID string) ([]model.PlaylistTracks, error) {
	var tracks []model.PlaylistTracks

	err := r.db.NewSelect().
		Model(&tracks).
		Where("spotify_id = ?", spotifyID).
		Order("added_at ASC").
		Scan(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to get playlist tracks: %w", err)
	}

	return tracks, nil
}

// GetRandomTrack retrieves a single random track from a playlist, excluding a list of forbidden IDs.
func (r *PlaylistTracksRepository) GetRandomTrack(ctx context.Context, spotifyID string, excludeTrackIDs []string) (*model.PlaylistTracks, error) {
	track := new(model.PlaylistTracks)

	r.logger.Info("GetRandomTrack called", zap.String("spotify_id", spotifyID), zap.Strings("exclude_track_ids", excludeTrackIDs))

	query := r.db.NewSelect().
		Model(track).
		Where("spotify_id = ?", spotifyID)

	// Exclude already issued tracks
	if len(excludeTrackIDs) > 0 {
		query = query.Where("track_id NOT IN (?)", bun.In(excludeTrackIDs))
	}

	err := query.
		OrderExpr("RANDOM()").
		Limit(1).
		Scan(ctx)

	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			r.logger.Info("No tracks found in playlist", zap.String("spotify_id", spotifyID))
			return nil, nil
		}
		r.logger.Error("Failed to get random track", zap.Error(err))
		return nil, fmt.Errorf("failed to get random track: %w", err)
	}

	r.logger.Info("Found random track", zap.String("track_id", track.TrackID), zap.String("artist", track.Artist), zap.String("title", track.Title))
	return track, nil
}

// Create inserts a new track record, updating metadata on conflict.
func (r *PlaylistTracksRepository) Create(ctx context.Context, track *model.PlaylistTracks) error {

	_, err := r.db.NewInsert().
		Model(track).
		On("CONFLICT (spotify_id, track_id) DO UPDATE").
		Set("artist = EXCLUDED.artist").
		Set("title = EXCLUDED.title").
		Set("updated_at = CURRENT_TIMESTAMP").
		Exec(ctx)

	if err != nil {
		return fmt.Errorf("failed to create playlist track: %w", err)
	}

	return nil
}

// Update modifies an existing track record.
func (r *PlaylistTracksRepository) Update(ctx context.Context, track *model.PlaylistTracks) error {

	_, err := r.db.NewUpdate().
		Model(track).
		Where("id = ?", track.ID).
		Exec(ctx)

	if err != nil {
		return fmt.Errorf("failed to update playlist track: %w", err)
	}

	return nil
}

// Delete removes a track record from the repository by its ID.
func (r *PlaylistTracksRepository) Delete(ctx context.Context, id int) error {

	_, err := r.db.NewDelete().
		Model((*model.PlaylistTracks)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	if err != nil {
		return fmt.Errorf("failed to delete playlist track: %w", err)
	}

	return nil
}

// DeleteBySpotifyID removes all tracks associated with a specific Spotify playlist.
func (r *PlaylistTracksRepository) DeleteBySpotifyID(ctx context.Context, spotifyID string) error {

	_, err := r.db.NewDelete().
		Model((*model.PlaylistTracks)(nil)).
		Where("spotify_id = ?", spotifyID).
		Exec(ctx)

	if err != nil {
		return fmt.Errorf("failed to delete playlist tracks by spotify_id: %w", err)
	}

	return nil
}

// GetAllBySpotifyID is an alias for GetBySpotifyID.
func (r *PlaylistTracksRepository) GetAllBySpotifyID(ctx context.Context, spotifyID string) ([]model.PlaylistTracks, error) {
	return r.GetBySpotifyID(ctx, spotifyID)
}
