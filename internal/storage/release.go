package storage

import (
	"context"
	"fmt"
	"gemfactory/internal/model"
	"time"

	"github.com/uptrace/bun"
	"go.uber.org/zap"
)

type ReleaseRepository struct {
	db     *bun.DB
	logger *zap.Logger
}

func NewReleaseRepository(db *bun.DB, logger *zap.Logger) *ReleaseRepository {
	return &ReleaseRepository{
		db:     db,
		logger: logger,
	}
}

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

func (r *ReleaseRepository) GetByGender(ctx context.Context, gender model.Gender) ([]model.Release, error) {
	var releases []model.Release
	err := r.db.NewSelect().
		Model(&releases).
		Relation("Artist").
		Where("artist.gender = ? AND release.is_active = true", gender).
		Order("date ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query releases by gender: %w", err)
	}
	return releases, nil
}

func (r *ReleaseRepository) GetByArtistID(ctx context.Context, artistID int) ([]model.Release, error) {
	var releases []model.Release
	err := r.db.NewSelect().
		Model(&releases).
		Relation("Artist").
		Where("release.artist_id = ? AND release.is_active = true", artistID).
		Order("date ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query releases by artist ID: %w", err)
	}
	return releases, nil
}

func (r *ReleaseRepository) GetByArtist(ctx context.Context, artistName string) ([]model.Release, error) {
	var releases []model.Release
	err := r.db.NewSelect().
		Model(&releases).
		Relation("Artist").
		Where("LOWER(artist.name) = LOWER(?) AND release.is_active = true", artistName).
		Order("date ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query releases by artist name: %w", err)
	}
	return releases, nil
}

func (r *ReleaseRepository) GetByDateRange(ctx context.Context, start, end time.Time) ([]model.Release, error) {
	var releases []model.Release
	err := r.db.NewSelect().
		Model(&releases).
		Relation("Artist").
		Where("date >= ? AND date <= ? AND release.is_active = true", start, end).
		Order("date ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query releases by date range: %w", err)
	}
	return releases, nil
}

func (r *ReleaseRepository) GetActive(ctx context.Context) ([]model.Release, error) {
	var releases []model.Release
	err := r.db.NewSelect().
		Model(&releases).
		Relation("Artist").
		Where("release.is_active = true").
		Order("date ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query active releases: %w", err)
	}
	return releases, nil
}

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

func (r *ReleaseRepository) Create(ctx context.Context, release *model.Release) error {
	_, err := r.db.NewInsert().
		Model(release).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to create release: %w", err)
	}
	return nil
}

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

func (r *ReleaseRepository) DeleteByIDs(ctx context.Context, ids []int) (int, error) {
	res, err := r.db.NewDelete().
		Model((*model.Release)(nil)).
		Where("release_id IN (?)", bun.List(ids)).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to delete releases by IDs: %w", err)
	}
	affected, _ := res.RowsAffected()
	return int(affected), nil
}

func (r *ReleaseRepository) Upsert(ctx context.Context, release *model.Release) error {
	_, err := r.db.NewInsert().
		Model(release).
		On("CONFLICT (artist_id, date, source_url) DO UPDATE").
		Set("display_artist = EXCLUDED.display_artist").
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
