// Package repository provides database-specific implementations of domain repositories.
package repository

import (
	"context"
	"fmt"
	"gemfactory/internal/model"

	"github.com/uptrace/bun"
	"go.uber.org/zap"
)

// ArtistRepository manages persistent storage for artist records using the Bun ORM.
type ArtistRepository struct {
	db     *bun.DB
	logger *zap.Logger
}

// NewArtistRepository initializes a new ArtistRepository.
func NewArtistRepository(db *bun.DB, logger *zap.Logger) *ArtistRepository {
	return &ArtistRepository{
		db:     db,
		logger: logger,
	}
}

// GetByID retrieves a single artist by its unique ID.
func (r *ArtistRepository) GetByID(ctx context.Context, id int) (*model.Artist, error) {
	artist := new(model.Artist)

	err := r.db.NewSelect().
		Model(artist).
		Where("artist_id = ?", id).
		Scan(ctx)

	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query artist by ID: %w", err)
	}

	return artist, nil
}

// GetAll retrieves all artist records, ordered alphabetically by name.
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

// GetByGender retrieves artists filtered by their gender.
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

// GetByName retrieves a single artist by its unique name.
func (r *ArtistRepository) GetByName(ctx context.Context, name string) (*model.Artist, error) {
	artist := new(model.Artist)

	err := r.db.NewSelect().
		Model(artist).
		Where("LOWER(name) = LOWER(?)", name).
		Scan(ctx)

	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query artist by name: %w", err)
	}

	return artist, nil
}

// GetActive retrieves all artists currently flagged as active.
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

// GetByGenderAndActive retrieves active artists matching a specific gender.
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

// Create inserts a new artist record into the database.
func (r *ArtistRepository) Create(ctx context.Context, artist *model.Artist) error {
	_, err := r.db.NewInsert().
		Model(artist).
		Exec(ctx)

	if err != nil {
		return fmt.Errorf("failed to create artist: %w", err)
	}

	return nil
}

// Update modifies an existing artist record.
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

// Delete removes an artist record by its ID.
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

// Upsert adds or updates multiple artist records based on conflict rules.
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

// DeactivateByNames updates multiple artists to is_active = false in a single query.
func (r *ArtistRepository) DeactivateByNames(ctx context.Context, names []string) (int, error) {
	if len(names) == 0 {
		return 0, nil
	}

	res, err := r.db.NewUpdate().
		Model((*model.Artist)(nil)).
		Set("is_active = false").
		Set("updated_at = CURRENT_TIMESTAMP").
		Where("name IN (?)", bun.In(names)).
		Where("is_active = true").
		Exec(ctx)

	if err != nil {
		return 0, fmt.Errorf("failed to deactivate artists: %w", err)
	}

	rowsAffected, _ := res.RowsAffected()
	return int(rowsAffected), nil
}
