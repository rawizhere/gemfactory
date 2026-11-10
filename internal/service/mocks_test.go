package service

import (
	"context"
	"gemfactory/internal/model"
	"time"
)

type mockReleaseRepo struct {
	model.ReleaseRepository

	byDateRange   []model.Release
	byArtist      []model.Release
	created       []*model.Release
	updated       []*model.Release
	deleted       []int
	errByDateRage error
}

func (m *mockReleaseRepo) GetByDateRange(ctx context.Context, start, end time.Time) ([]model.Release, error) {
	return m.byDateRange, m.errByDateRage
}

func (m *mockReleaseRepo) GetByArtistID(ctx context.Context, artistID int) ([]model.Release, error) {
	return m.byArtist, nil
}

func (m *mockReleaseRepo) Create(ctx context.Context, release *model.Release) error {
	m.created = append(m.created, release)
	return nil
}

func (m *mockReleaseRepo) Update(ctx context.Context, release *model.Release) error {
	m.updated = append(m.updated, release)
	return nil
}

func (m *mockReleaseRepo) Delete(ctx context.Context, id int) error {
	m.deleted = append(m.deleted, id)
	return nil
}
