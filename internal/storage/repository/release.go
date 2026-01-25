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
		Order("date ASC").
		Scan(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to query releases: %w", err)
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

// GetByArtistDateAndTrack retrieves a release by its artist, exact date, and track title.
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

// GetByArtistDateAndYouTube: Retrieves a release by its artist, date, and source video URL.
func (r *ReleaseRepository) GetByArtistDateAndYouTube(ctx context.Context, artistID int, date time.Time, youtubeURL string) (*model.Release, error) {
	var release model.Release

	// If YouTube URL is empty, match only by artist and date with empty MV.
	if youtubeURL == "" || youtubeURL == "N/A" {
		err := r.db.NewSelect().
			Model(&release).
			Where("artist_id = ? AND date = ?", artistID, date).
			Where("(mv = '' OR mv = 'N/A')").
			Scan(ctx)

		if err != nil {
			if err.Error() == "sql: no rows in result set" {
				return nil, nil
			}
			return nil, fmt.Errorf("failed to query release by artist and date: %w", err)
		}
		return &release, nil
	}

	err := r.db.NewSelect().
		Model(&release).
		Where("artist_id = ? AND date = ? AND mv = ?", artistID, date, youtubeURL).
		Scan(ctx)

	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query release by artist, date and youtube: %w", err)
	}

	return &release, nil
}

// GetByArtistDateAndSource: Retrieves a release by its artist, date, and original source URL.
func (r *ReleaseRepository) GetByArtistDateAndSource(ctx context.Context, artistID int, date time.Time, sourceURL string) (*model.Release, error) {
	var release model.Release

	if sourceURL == "" {
		return nil, nil
	}

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

// GetByGender: Retrieves active releases for artists of a specific gender.
func (r *ReleaseRepository) GetByGender(ctx context.Context, gender model.Gender) ([]model.Release, error) {
	var releases []model.Release

	err := r.db.NewSelect().
		Model(&releases).
		Relation("Artist").
		Where("artist.gender = ?", gender).
		Where("artist.is_active = ?", true).
		Order("release.date ASC").
		Scan(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to query releases by gender: %w", err)
	}

	return releases, nil
}

// Create: Inserts a new release record if it does not already exist.
func (r *ReleaseRepository) Create(ctx context.Context, release *model.Release) error {
	// Combination of ArtistID, Date, and Title must be unique.
	exists, err := r.db.NewSelect().
		Model((*model.Release)(nil)).
		Where("artist_id = ? AND date = ? AND title = ?", release.ArtistID, release.Date, release.Title).
		Exists(ctx)

	if err != nil {
		return fmt.Errorf("failed to check existing release: %w", err)
	}

	if exists {
		return nil
	}

	_, err = r.db.NewInsert().
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

// Delete removes a release record from the repository by its ID.
func (r *ReleaseRepository) Delete(ctx context.Context, id int) error {
	_, err := r.db.NewDelete().
		Model((*model.Release)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	if err != nil {
		return fmt.Errorf("failed to delete release: %w", err)
	}

	return nil
}

// GetByArtistID retrieves all releases for a specific artist, including artist details.
func (r *ReleaseRepository) GetByArtistID(ctx context.Context, artistID int) ([]model.Release, error) {
	var releases []model.Release

	err := r.db.NewSelect().
		Model(&releases).
		Relation("Artist").
		Where("artist_id = ?", artistID).
		Order("date ASC").
		Scan(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to query releases by artist: %w", err)
	}

	return releases, nil
}

// GetByArtist searches for releases by the artist's name, returning only active records.
func (r *ReleaseRepository) GetByArtist(ctx context.Context, artistName string) ([]model.Release, error) {
	var releases []model.Release

	err := r.db.NewSelect().
		Model(&releases).
		Relation("Artist").
		Where("LOWER(artist.name) = LOWER(?)", artistName).
		Where("release.is_active = ?", true).
		Where("artist.is_active = ?", true).
		Order("date ASC").
		Scan(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to query releases by artist name: %w", err)
	}

	return releases, nil
}

// GetByDateRange retrieves all releases within a specific timeframe, inclusive.
func (r *ReleaseRepository) GetByDateRange(ctx context.Context, start, end time.Time) ([]model.Release, error) {
	var releases []model.Release

	err := r.db.NewSelect().
		Model(&releases).
		Relation("Artist").
		Where("release.date >= ? AND release.date <= ?", start, end).
		Order("release.date ASC").
		Scan(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to query releases by date range: %w", err)
	}

	return releases, nil
}

// GetActive retrieves all currently active releases across all artists.
func (r *ReleaseRepository) GetActive(ctx context.Context) ([]model.Release, error) {
	var releases []model.Release

	err := r.db.NewSelect().
		Model(&releases).
		Relation("Artist").
		Where("release.is_active = ?", true).
		Order("release.date ASC").
		Scan(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to query active releases: %w", err)
	}

	return releases, nil
}

// GetWithRelations retrieves active releases joined with active artist data.
func (r *ReleaseRepository) GetWithRelations(ctx context.Context) ([]model.Release, error) {
	var releases []model.Release

	err := r.db.NewSelect().
		Model(&releases).
		Relation("Artist").
		Where("release.is_active = ?", true).
		Where("artist.is_active = ?", true).
		Order("date ASC").
		Scan(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to query releases with relations: %w", err)
	}

	return releases, nil
}

// GetTotalCount returns the total number of release records in the repository.
func (r *ReleaseRepository) GetTotalCount(ctx context.Context) (int, error) {
	count, err := r.db.NewSelect().
		Model((*model.Release)(nil)).
		Count(ctx)

	if err != nil {
		return 0, fmt.Errorf("failed to count releases: %w", err)
	}

	return count, nil
}
