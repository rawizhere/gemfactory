// Package repository provides database-specific implementations of domain repositories.
package repository

import (
	"context"
	"fmt"
	"gemfactory/internal/model"
	"time"

	"github.com/uptrace/bun"
	"go.uber.org/zap"
)

// ReleaseRepository manages persistent storage for music release records.
type ReleaseRepository struct {
	db     *bun.DB
	logger *zap.Logger
}

// NewReleaseRepository initializes a new ReleaseRepository.
func NewReleaseRepository(db *bun.DB, logger *zap.Logger) *ReleaseRepository {
	return &ReleaseRepository{
		db:     db,
		logger: logger,
	}
}

// GetByID retrieves a single release record by its unique ID, including artist details.
func (r *ReleaseRepository) GetByID(ctx context.Context, id int) (*model.Release, error) {
	release := new(model.Release)

	err := r.db.NewSelect().
		Model(release).
		Relation("Artist").
		Where("release_id = ?", id).
		Scan(ctx)

	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query release by ID: %w", err)
	}

	return release, nil
}

// GetAll retrieves all release records, ordered chronologically.
func (r *ReleaseRepository) GetAll(ctx context.Context) ([]model.Release, error) {
	var releases []model.Release
	err := r.db.NewSelect().
		Model(&releases).
		Relation("Artist").
		Order("date ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query all releases: %w", err)
	}
	return releases, nil
}

// GetByGender retrieves releases filtered by the artist's gender.
func (r *ReleaseRepository) GetByGender(ctx context.Context, gender model.Gender) ([]model.Release, error) {
	var releases []model.Release
	err := r.db.NewSelect().
		Model(&releases).
		Relation("Artist").
		Where("artist.gender = ? AND releases.is_active = true", gender).
		Order("date ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query releases by gender: %w", err)
	}
	return releases, nil
}

// GetByArtistID retrieves all releases associated with a specific artist ID.
func (r *ReleaseRepository) GetByArtistID(ctx context.Context, artistID int) ([]model.Release, error) {
	var releases []model.Release
	err := r.db.NewSelect().
		Model(&releases).
		Relation("Artist").
		Where("artist_id = ? AND is_active = true", artistID).
		Order("date ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query releases by artist ID: %w", err)
	}
	return releases, nil
}

// GetByArtist retrieves all releases matching an artist's name (case-insensitive).
func (r *ReleaseRepository) GetByArtist(ctx context.Context, artistName string) ([]model.Release, error) {
	var releases []model.Release
	err := r.db.NewSelect().
		Model(&releases).
		Relation("Artist").
		Where("LOWER(artist.name) = LOWER(?) AND releases.is_active = true", artistName).
		Order("date ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query releases by artist name: %w", err)
	}
	return releases, nil
}

// GetByDateRange retrieves releases within a specific timeframe.
func (r *ReleaseRepository) GetByDateRange(ctx context.Context, start, end time.Time) ([]model.Release, error) {
	var releases []model.Release
	err := r.db.NewSelect().
		Model(&releases).
		Relation("Artist").
		Where("date >= ? AND date <= ? AND releases.is_active = true", start, end).
		Order("date ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query releases by date range: %w", err)
	}
	return releases, nil
}

// GetActive retrieves all active releases.
func (r *ReleaseRepository) GetActive(ctx context.Context) ([]model.Release, error) {
	var releases []model.Release
	err := r.db.NewSelect().
		Model(&releases).
		Relation("Artist").
		Where("releases.is_active = true").
		Order("date ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query active releases: %w", err)
	}
	return releases, nil
}

// GetByArtistAndTitle retrieves a single release by its artist and exact title.
func (r *ReleaseRepository) GetByArtistAndTitle(ctx context.Context, artistID int, title string) (*model.Release, error) {
	var release model.Release
	err := r.db.NewSelect().
		Model(&release).
		Where("artist_id = ? AND title = ?", artistID, title).
		Scan(ctx)

	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query release by artist and title: %w", err)
	}
	return &release, nil
}

// GetByArtistDateAndTrack retrieves a release by its artist, date, and title track name.
func (r *ReleaseRepository) GetByArtistDateAndTrack(ctx context.Context, artistID int, date time.Time, titleTrack string) (*model.Release, error) {
	var release model.Release
	err := r.db.NewSelect().
		Model(&release).
		Where("artist_id = ? AND date = ? AND title_track = ?", artistID, date, titleTrack).
		Scan(ctx)

	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query release by artist, date and track: %w", err)
	}
	return &release, nil
}

// GetByArtistDateAndSource retrieves a release by its artist, date, and origin URL.
func (r *ReleaseRepository) GetByArtistDateAndSource(ctx context.Context, artistID int, date time.Time, sourceURL string) (*model.Release, error) {
	var release model.Release
	err := r.db.NewSelect().
		Model(&release).
		Where("artist_id = ? AND date = ? AND source_url = ?", artistID, date, sourceURL).
		Scan(ctx)

	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query release by artist, date and source: %w", err)
	}
	return &release, nil
}

// GetTotalCount returns the total count of active releases.
func (r *ReleaseRepository) GetTotalCount(ctx context.Context) (int, error) {
	count, err := r.db.NewSelect().
		Model((*model.Release)(nil)).
		Where("is_active = true").
		Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to count releases: %w", err)
	}
	return count, nil
}

// Create inserts a single release.
func (r *ReleaseRepository) Create(ctx context.Context, release *model.Release) error {
	_, err := r.db.NewInsert().
		Model(release).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to create release: %w", err)
	}
	return nil
}

// Update modifies an existing release record.
func (r *ReleaseRepository) Update(ctx context.Context, release *model.Release) error {
	_, err := r.db.NewUpdate().
		Model(release).
		WherePK().
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update release: %w", err)
	}
	return nil
}

// Delete removes a release record.
func (r *ReleaseRepository) Delete(ctx context.Context, id int) error {
	_, err := r.db.NewDelete().
		Model((*model.Release)(nil)).
		Where("release_id = ?", id).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete release: %w", err)
	}
	return nil
}

// Upsert adds or updates a release record.
func (r *ReleaseRepository) Upsert(ctx context.Context, release *model.Release) error {
	_, err := r.db.NewInsert().
		Model(release).
		On("CONFLICT (artist_id, date, source_url) DO UPDATE").
		Set("title = EXCLUDED.title").
		Set("title_track = EXCLUDED.title_track").
		Set("album_name = EXCLUDED.album_name").
		Set("mv = EXCLUDED.mv").
		Set("spotify = EXCLUDED.spotify").
		Set("is_active = true").
		Set("updated_at = CURRENT_TIMESTAMP").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to upsert release: %w", err)
	}
	return nil
}
