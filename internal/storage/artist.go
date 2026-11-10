package storage

import (
	"context"
	"fmt"
	"gemfactory/internal/model"

	"github.com/uptrace/bun"
)

type ArtistRepository struct {
	db *bun.DB
}

func NewArtistRepository(db *bun.DB) model.ArtistRepository {
	return &ArtistRepository{db: db}
}

func (r *ArtistRepository) GetByID(ctx context.Context, id int) (*model.Artist, error) {
	return one(ctx, r.db.NewSelect().Where("artist_id = ?", id), new(model.Artist), "artist by ID")
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

func (r *ArtistRepository) GetByName(ctx context.Context, name string) (*model.Artist, error) {
	return one(ctx, r.db.NewSelect().Where("LOWER(name) = LOWER(?)", name), new(model.Artist), "artist by name")
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
