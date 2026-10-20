package service

import (
	"context"
	"fmt"
	"gemfactory/internal/model"
	"gemfactory/internal/storage"
	"slices"
	"strings"

	"github.com/uptrace/bun"
)

type ArtistService struct {
	repo model.ArtistRepository
}

func NewArtistService(db *bun.DB) *ArtistService {
	return &ArtistService{repo: storage.NewArtistRepository(db)}
}

func (s *ArtistService) Add(ctx context.Context, artists []string, isFemale bool) (int, error) {
	if len(artists) == 0 {
		return 0, nil
	}

	var models []model.Artist
	for _, artistName := range artists {
		models = append(models, model.Artist{
			Name:     (strings.TrimSpace(artistName)),
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

func (s *ArtistService) Export(ctx context.Context) (string, error) {
	allArtists, err := s.repo.GetAll(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get all artists: %w", err)
	}
	return s.formatArtists(allArtists), nil
}

func (s *ArtistService) FormatList(ctx context.Context) (string, error) {
	artists, err := s.repo.GetActive(ctx)
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
			femaleArtists = append(femaleArtists, artist.Name)
		} else {
			maleArtists = append(maleArtists, artist.Name)
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

	fmt.Fprintf(&response, "\nTotal Artists: %d\nFemale: %d\nMale: %d",
		len(femaleArtists)+len(maleArtists), len(femaleArtists), len(maleArtists))

	return response.String()
}

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
