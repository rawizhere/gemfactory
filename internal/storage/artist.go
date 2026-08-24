package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"gemfactory/internal/model"

	"github.com/uptrace/bun"
	"go.uber.org/zap"
)

type ArtistRepository struct {
	db     *bun.DB
	logger *zap.Logger
}

func NewArtistRepository(db *bun.DB, logger *zap.Logger) *ArtistRepository {
	return &ArtistRepository{
		db:     db,
		logger: logger,
	}
}

func (r *ArtistRepository) GetByID(ctx context.Context, id int) (*model.Artist, error) {
	artist := new(model.Artist)

	err := r.db.NewSelect().
		Model(artist).
		Where("artist_id = ?", id).
		Scan(ctx)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query artist by ID: %w", err)
	}

	return artist, nil
}

func (r *ArtistRepository) GetAll(ctx context.Context) ([]model.Artist, error) {
	var artists []model.Artist
	err := r.db.NewSelect().
		Model(&artists).
		Order("name ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query all artists: %w", err)
	}
	return artists, nil
}

func (r *ArtistRepository) GetByGender(ctx context.Context, gender model.Gender) ([]model.Artist, error) {
	var artists []model.Artist
	err := r.db.NewSelect().
		Model(&artists).
		Where("gender = ?", gender).
		Order("name ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query artists by gender: %w", err)
	}
	return artists, nil
}

func (r *ArtistRepository) GetByName(ctx context.Context, name string) (*model.Artist, error) {
	artist := new(model.Artist)

	err := r.db.NewSelect().
		Model(artist).
		Where("LOWER(name) = LOWER(?)", name).
		Scan(ctx)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query artist by name: %w", err)
	}

	return artist, nil
}

func (r *ArtistRepository) GetActive(ctx context.Context) ([]model.Artist, error) {
	var artists []model.Artist
	err := r.db.NewSelect().
		Model(&artists).
		Where("is_active = true").
		Order("name ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query active artists: %w", err)
	}
	return artists, nil
}

func (r *ArtistRepository) GetByGenderAndActive(ctx context.Context, gender model.Gender, active bool) ([]model.Artist, error) {
	var artists []model.Artist
	err := r.db.NewSelect().
		Model(&artists).
		Where("gender = ? AND is_active = ?", gender, active).
		Order("name ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query artists by gender and active: %w", err)
	}
	return artists, nil
}

func (r *ArtistRepository) Create(ctx context.Context, artist *model.Artist) error {
	_, err := r.db.NewInsert().
		Model(artist).
		Exec(ctx)

	if err != nil {
		return fmt.Errorf("failed to create artist: %w", err)
	}

	return nil
}

func (r *ArtistRepository) Update(ctx context.Context, artist *model.Artist) error {
	_, err := r.db.NewUpdate().
		Model(artist).
		WherePK().
		Exec(ctx)

	if err != nil {
		return fmt.Errorf("failed to update artist: %w", err)
	}

	return nil
}

func (r *ArtistRepository) Delete(ctx context.Context, id int) error {
	_, err := r.db.NewDelete().
		Model((*model.Artist)(nil)).
		Where("artist_id = ?", id).
		Exec(ctx)

	if err != nil {
		return fmt.Errorf("failed to delete artist: %w", err)
	}

	return nil
}

func (r *ArtistRepository) Upsert(ctx context.Context, artists []model.Artist) error {
	if len(artists) == 0 {
		return nil
	}

	_, err := r.db.NewInsert().
		Model(&artists).
		On("CONFLICT (name) DO UPDATE").
		Set("is_active = true").
		Set("gender = EXCLUDED.gender").
		Set("updated_at = CURRENT_TIMESTAMP").
		Exec(ctx)

	if err != nil {
		return fmt.Errorf("failed to upsert artists: %w", err)
	}

	return nil
}

func (r *ArtistRepository) DeactivateByNames(ctx context.Context, names []string) (int, error) {
	if len(names) == 0 {
		return 0, nil
	}

	res, err := r.db.NewUpdate().
		Model((*model.Artist)(nil)).
		Set("is_active = false").
		Set("updated_at = CURRENT_TIMESTAMP").
		Where("name IN (?)", bun.List(names)).
		Where("is_active = true").
		Exec(ctx)

	if err != nil {
		return 0, fmt.Errorf("failed to deactivate artists: %w", err)
	}

	rowsAffected, _ := res.RowsAffected()
	return int(rowsAffected), nil
}
