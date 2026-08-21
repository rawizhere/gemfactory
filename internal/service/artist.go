// Package service implements the core business logic and domain services.
package service

import (
	"context"
	"fmt"
	"gemfactory/internal/model"
	"gemfactory/internal/storage/repository"
	"slices"
	"strings"

	"github.com/uptrace/bun"
	"go.uber.org/zap"
)

// ArtistService provides methods for managing and formatting music artists.
type ArtistService struct {
	repo   model.ArtistRepository
	logger *zap.Logger
}

func NewArtistService(db *bun.DB, logger *zap.Logger) *ArtistService {
	return &ArtistService{
		repo:   repository.NewArtistRepository(db, logger),
		logger: logger,
	}
}

// Add inserts new artists into the repository.
func (s *ArtistService) Add(ctx context.Context, artists []string, isFemale bool) (int, error) {
	if len(artists) == 0 {
		return 0, nil
	}

	var models []model.Artist
	for _, artistName := range artists {
		models = append(models, model.Artist{
			Name:     model.NewUniqueString(strings.TrimSpace(artistName)),
			Gender:   model.FromBool(isFemale),
			IsActive: true,
		})
	}

	err := s.repo.Upsert(ctx, models)
	if err != nil {
		return 0, fmt.Errorf("failed to add artists: %w", err)
	}

	return len(models), nil
}

// Remove deletes artists from the repository by name.
func (s *ArtistService) Remove(ctx context.Context, artists []string) (int, error) {
	removedCount := 0
	for _, artistName := range artists {
		artist, err := s.repo.GetByName(ctx, artistName)
		if err != nil {
			return removedCount, fmt.Errorf("failed to get artist %s: %w", artistName, err)
		}

		if artist == nil {
			s.logger.Warn("Artist not found", zap.String("artist", artistName))
			continue
		}

		err = s.repo.Delete(ctx, artist.ArtistID)
		if err != nil {
			return removedCount, fmt.Errorf("failed to delete artist %s: %w", artistName, err)
		}
		removedCount++
	}

	return removedCount, nil
}

// Deactivate marks artists as inactive in a single batch query.
func (s *ArtistService) Deactivate(ctx context.Context, artists []string) (int, error) {
	if len(artists) == 0 {
		return 0, nil
	}

	cleanNames := make([]string, 0, len(artists))
	for _, a := range artists {
		if trimmed := strings.TrimSpace(a); trimmed != "" {
			cleanNames = append(cleanNames, trimmed)
		}
	}

	return s.repo.DeactivateByNames(ctx, cleanNames)
}

// GetFemaleArtists retrieves names of all active female artists.
func (s *ArtistService) GetFemaleArtists(ctx context.Context) ([]string, error) {
	artists, err := s.repo.GetByGenderAndActive(ctx, model.GenderFemale, true)
	if err != nil {
		return nil, fmt.Errorf("failed to get female artists: %w", err)
	}

	names := make([]string, 0, len(artists))
	for _, artist := range artists {
		names = append(names, artist.Name.String())
	}
	return names, nil
}

// GetMaleArtists retrieves names of all active male artists.
func (s *ArtistService) GetMaleArtists(ctx context.Context) ([]string, error) {
	artists, err := s.repo.GetByGenderAndActive(ctx, model.GenderMale, true)
	if err != nil {
		return nil, fmt.Errorf("failed to get male artists: %w", err)
	}

	names := make([]string, 0, len(artists))
	for _, artist := range artists {
		names = append(names, artist.Name.String())
	}
	return names, nil
}

// GetAll retrieves all artist records.
func (s *ArtistService) GetAll(ctx context.Context) ([]model.Artist, error) {
	return s.repo.GetAll(ctx)
}

// GetAllActive retrieves all active artist records.
func (s *ArtistService) GetAllActive(ctx context.Context) ([]model.Artist, error) {
	return s.repo.GetActive(ctx)
}

// Export returns a formatted string of all artists for administrative use.
func (s *ArtistService) Export(ctx context.Context) (string, error) {
	allArtists, err := s.GetAll(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get all artists: %w", err)
	}
	return s.formatArtists(allArtists), nil
}

// FormatList returns a human-readable list of active artists grouped by gender.
func (s *ArtistService) FormatList(ctx context.Context) (string, error) {
	artists, err := s.GetAllActive(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get active artists: %w", err)
	}
	return s.formatArtists(artists), nil
}

func (s *ArtistService) formatArtists(artists []model.Artist) string {
	var femaleArtists []string
	var maleArtists []string

	for _, artist := range artists {
		if artist.IsFemale() {
			femaleArtists = append(femaleArtists, artist.Name.String())
		} else {
			maleArtists = append(maleArtists, artist.Name.String())
		}
	}

	var response strings.Builder
	response.WriteString("<b>Female Artists:</b>\n")
	if len(femaleArtists) == 0 {
		response.WriteString("empty\n")
	} else {
		slices.Sort(femaleArtists)
		fmt.Fprintf(&response, "<code>%s</code>\n", strings.Join(femaleArtists, ", "))
	}

	response.WriteString("\n<b>Male Artists:</b>\n")
	if len(maleArtists) == 0 {
		response.WriteString("empty\n")
	} else {
		slices.Sort(maleArtists)
		fmt.Fprintf(&response, "<code>%s</code>\n", strings.Join(maleArtists, ", "))
	}

	fmt.Fprintf(&response, "\n📊 Total Artists: %d\n💃 Female: %d\n🤦‍♂️ Male: %d",
		len(femaleArtists)+len(maleArtists), len(femaleArtists), len(maleArtists))

	return response.String()
}

// GetCounts returns the number of active female, male, and total artists.
func (s *ArtistService) GetCounts(ctx context.Context) (femaleCount, maleCount, totalCount int, err error) {
	artists, err := s.repo.GetActive(ctx)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to get active artists: %w", err)
	}

	for _, artist := range artists {
		switch artist.Gender {
		case model.GenderFemale:
			femaleCount++
		case model.GenderMale:
			maleCount++
		}
	}

	totalCount = femaleCount + maleCount
	return femaleCount, maleCount, totalCount, nil
}
