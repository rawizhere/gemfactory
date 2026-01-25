// Package service contains business logic.
package service

import (
	"context"
	"fmt"
	"gemfactory/internal/model"
	"gemfactory/internal/scraper"
	"gemfactory/internal/storage/repository"
	"gemfactory/internal/validator"
	"strconv"
	"strings"
	"time"

	"github.com/uptrace/bun"
	"go.uber.org/zap"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// ReleaseService coordinates the collection, filtering, and presentation of music releases.
type ReleaseService struct {
	repo       model.ReleaseRepository
	artistRepo model.ArtistRepository
	scraper    scraper.Fetcher
	logger     *zap.Logger
}

func NewReleaseService(db *bun.DB, scraper scraper.Fetcher, logger *zap.Logger) *ReleaseService {
	return &ReleaseService{
		repo:       repository.NewReleaseRepository(db, logger),
		artistRepo: repository.NewArtistRepository(db, logger),
		scraper:    scraper,
		logger:     logger,
	}
}

// GetReleasesForMonth retrieves and formats all releases within a specified calendar month.
func (s *ReleaseService) GetReleasesForMonth(ctx context.Context, month string, femaleOnly, maleOnly bool) (string, error) {
	month = strings.ToLower(month)

	var year int
	if strings.Contains(month, "-") {
		parts := strings.Split(month, "-")
		if len(parts) == 2 {
			month = parts[0]
			if parsedYear, err := strconv.Atoi(parts[1]); err == nil {
				year = parsedYear
			}
		}
	}

	// ! Handle year crossover logic if month is "January" but we are in "December".
	if strings.ToLower(month) == "january" && time.Now().Month() == time.December {
		nextYear := time.Now().AddDate(1, 0, 0).Year()
		if year == 0 || year < nextYear { // Only update if year wasn't explicitly provided or is in the past.
			year = nextYear
		}
	}

	if year == 0 {
		year = time.Now().Year()
	}

	var gender string
	if femaleOnly {
		gender = "female"
	} else if maleOnly {
		gender = "male"
	}

	startDate := time.Date(year, time.Month(monthToInt(month)), 1, 0, 0, 0, 0, time.UTC)
	endDate := startDate.AddDate(0, 1, -1)

	allReleases, err := s.repo.GetByDateRange(ctx, startDate, endDate)
	if err != nil {
		return "", fmt.Errorf("failed to get releases by date range: %w", err)
	}

	s.logger.Info("Retrieved releases for filtering",
		zap.String("month", month),
		zap.Int("year", year),
		zap.String("gender", gender),
		zap.Int("found_in_range", len(allReleases)))

	var releases []model.Release
	for _, release := range allReleases {
		if gender == "" || (release.Artist != nil && strings.ToLower(string(release.Artist.Gender)) == gender) {
			releases = append(releases, release)
		}
	}

	s.logger.Info("Filtered releases",
		zap.String("month", month),
		zap.Int("year", year),
		zap.String("gender", gender),
		zap.Int("filtered_count", len(releases)))

	var result strings.Builder

	caser := cases.Title(language.English)
	if year > 0 {
		result.WriteString(fmt.Sprintf("🎵 Releases for %s %d:\n\n", caser.String(month), year))
	} else {
		result.WriteString(fmt.Sprintf("🎵 Releases for %s:\n\n", caser.String(month)))
	}

	if len(releases) == 0 {
		result.WriteString("No releases found")
		return result.String(), nil
	}

	for _, release := range releases {
		line := FormatReleaseForTelegram(&release)
		result.WriteString(line + "\n")
	}

	return result.String(), nil
}

// Upsert updates an existing release record or creates a new one if it does not exist.
func (s *ReleaseService) Upsert(ctx context.Context, release *model.Release) error {
	if err := s.validateRelease(release); err != nil {
		return err
	}

	release.Title = CleanReleaseTitle(release.Title)
	release.AlbumName = CleanReleaseTitle(release.AlbumName)
	release.TitleTrack = CleanReleaseTitle(release.TitleTrack)

	// Filter common metadata noise from track titles.
	genericTitles := []string{
		"youtube", "official audio", "music video", "mv release",
		"jyp entertainment official youtube", "official youtube",
		"mv", "audio", "video",
	}

	titleLower := strings.ToLower(release.TitleTrack)
	for _, generic := range genericTitles {
		if titleLower == generic || strings.Contains(titleLower, generic) {
			release.TitleTrack = ""
			break
		}
	}

	var existingRelease *model.Release
	var err error

	// Try to find existing release by stable ID (SourceURL).
	if release.SourceURL != "" {
		existingRelease, err = s.repo.GetByArtistDateAndSource(ctx, release.ArtistID, release.Date, release.SourceURL)
	}

	// Fallback to searching by YouTube video URL.
	if err == nil && existingRelease == nil {
		if release.MV != "" && release.MV != "N/A" {
			existingRelease, err = s.repo.GetByArtistDateAndYouTube(ctx, release.ArtistID, release.Date, release.MV)
		}
	}

	// Fallback to searching by track title.
	if err == nil && existingRelease == nil {
		existingRelease, err = s.repo.GetByArtistDateAndTrack(ctx, release.ArtistID, release.Date, release.TitleTrack)
	}

	if err != nil {
		return fmt.Errorf("failed to check for existing release: %w", err)
	}

	if existingRelease != nil {
		s.logger.Info("Release exists, updating",
			zap.Int("artist_id", release.ArtistID),
			zap.String("date", release.Date.Format("02.01.06")),
			zap.String("track", release.TitleTrack))

		if release.Title != "" && release.Title != "N/A" {
			existingRelease.Title = release.Title
		}
		if release.AlbumName != "" && release.AlbumName != "N/A" {
			existingRelease.AlbumName = release.AlbumName
		}
		if release.TitleTrack != "" && release.TitleTrack != "N/A" {
			existingRelease.TitleTrack = release.TitleTrack
		}
		if release.MV != "" && release.MV != "N/A" {
			existingRelease.MV = release.MV
		}
		if release.Spotify != "" && release.Spotify != "N/A" {
			existingRelease.Spotify = release.Spotify
		}
		if release.SourceURL != "" {
			existingRelease.SourceURL = release.SourceURL
		}
		existingRelease.UpdatedAt = time.Now()

		return s.repo.Update(ctx, existingRelease)
	}

	s.logger.Info("Release not found, creating new",
		zap.Int("artist_id", release.ArtistID),
		zap.String("date", release.Date.Format("02.01.06")),
		zap.String("track", release.TitleTrack))

	return s.repo.Create(ctx, release)
}

// Update modifies an existing release record in the repository.
func (s *ReleaseService) Update(ctx context.Context, release *model.Release) error {
	if err := s.validateRelease(release); err != nil {
		return err
	}

	release.Title = CleanReleaseTitle(release.Title)
	release.AlbumName = CleanReleaseTitle(release.AlbumName)
	release.TitleTrack = CleanReleaseTitle(release.TitleTrack)

	return s.repo.Update(ctx, release)
}

func (s *ReleaseService) validateRelease(release *model.Release) error {
	var vErrors validator.ValidationErrors
	if release.ArtistID <= 0 {
		vErrors = append(vErrors, validator.ValidationError{Field: "artist_id", Message: "artist_id is required"})
	}
	if err := validator.ValidateRequired("title", release.Title); err != nil {
		vErrors = append(vErrors, err.(validator.ValidationError))
	}
	if release.Date.IsZero() {
		vErrors = append(vErrors, validator.ValidationError{Field: "date", Message: "date is required"})
	}
	if release.MV != "" {
		if err := validator.ValidateURL("mv", release.MV); err != nil {
			vErrors = append(vErrors, err.(validator.ValidationError))
		}
	}
	if vErrors.HasErrors() {
		return fmt.Errorf("release validation failed: %w", vErrors)
	}
	return nil
}

// FormatReleaseForDisplay provides a detailed multi-line representation of a release.
func (s *ReleaseService) FormatReleaseForDisplay(release *model.Release) string {
	return FormatReleaseForDisplay(release)
}

// FormatDate parses and re-formats a date string.
func (s *ReleaseService) FormatDate(dateStr string) (string, error) {
	parsedDate, err := ParseReleaseDate(dateStr)
	if err != nil {
		return "", err
	}
	return parsedDate.Format("02.01.06"), nil
}

// FormatTimeKST parses and formats a KST time string.
func (s *ReleaseService) FormatTimeKST(timeStr string) (string, error) {
	parsedTime, err := ParseReleaseTime(timeStr)
	if err != nil {
		return "", err
	}
	return parsedTime.Format("15:04"), nil
}

// ConvertKSTToMSK shifts a KST time to Moscow Standard Time (MSK).
func (s *ReleaseService) ConvertKSTToMSK(kstTimeStr string) (string, error) {
	parsedTime, err := ParseReleaseTime(kstTimeStr)
	if err != nil {
		return "", err
	}
	mskTime := parsedTime.Add(-6 * time.Hour)
	return mskTime.Format("15:04"), nil
}

// CleanLink removes unwanted metadata or channel links from YouTube URLs.
func (s *ReleaseService) CleanLink(link string) string {
	if isYouTubeChannel(link) {
		return ""
	}
	return CleanLink(link)
}

func isYouTubeChannel(link string) bool {
	low := strings.ToLower(link)
	return strings.Contains(low, "youtube.com/@") ||
		strings.Contains(low, "youtube.com/channel/") ||
		strings.Contains(low, "youtube.com/user/") ||
		strings.Contains(low, "youtube.com/c/")
}

// FormatReleaseForTelegram provides a concise single-line representation of a release for mobile viewing.
func (s *ReleaseService) FormatReleaseForTelegram(release *model.Release) string {
	return FormatReleaseForTelegram(release)
}

// Create adds a new release record to the repository.
func (s *ReleaseService) Create(ctx context.Context, release *model.Release) error {
	existing, err := s.repo.GetByArtistID(ctx, release.ArtistID)
	if err != nil {
		return fmt.Errorf("failed to check existing releases: %w", err)
	}

	for _, existingRelease := range existing {
		if existingRelease.Title == release.Title && existingRelease.Date.Equal(release.Date) {
			var artistName string
			if release.Artist != nil {
				artistName = release.Artist.Name
			}
			return fmt.Errorf("release already exists: %s - %s", artistName, release.Title)
		}
	}

	err = s.repo.Create(ctx, release)
	if err != nil {
		return fmt.Errorf("failed to create release: %w", err)
	}

	return nil
}

// Delete removes a release record from the repository by its ID.
func (s *ReleaseService) Delete(ctx context.Context, id int) error {
	err := s.repo.Delete(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to delete release: %w", err)
	}

	return nil
}

// GetByArtistID retrieves all releases associated with a specific artist ID.
func (s *ReleaseService) GetByArtistID(ctx context.Context, artistID int) ([]model.Release, error) {
	releases, err := s.repo.GetByArtistID(ctx, artistID)
	if err != nil {
		return nil, fmt.Errorf("failed to get releases by artist ID %d: %w", artistID, err)
	}

	return releases, nil
}

// GetReleasesByGender retrieves releases filtered by the artist's gender.
func (s *ReleaseService) GetReleasesByGender(ctx context.Context, gender string) ([]model.Release, error) {
	var genderType model.Gender
	switch gender {
	case "female":
		genderType = model.GenderFemale
	case "male":
		genderType = model.GenderMale
	default:
		genderType = model.GenderMixed
	}

	releases, err := s.repo.GetByGender(ctx, genderType)
	if err != nil {
		return nil, fmt.Errorf("failed to get releases by gender %s: %w", gender, err)
	}

	return releases, nil
}

// GetAllReleases retrieves all release records from the repository.
func (s *ReleaseService) GetAllReleases(ctx context.Context) ([]model.Release, error) {
	releases, err := s.repo.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get all releases: %w", err)
	}

	return releases, nil
}

// ParseReleasesForMonth initiates a scraping job to collect and save releases for a given month.
func (s *ReleaseService) ParseReleasesForMonth(ctx context.Context, monthName string) (int, error) {
	s.logger.Info("Starting to parse releases", zap.String("month", monthName))

	year := time.Now().Format("2006")
	month := monthName
	if strings.Contains(monthName, "-") {
		parts := strings.Split(monthName, "-")
		if len(parts) == 2 {
			month = parts[0]
			year = parts[1]
		}
	} else {
		// Handle new year crossover logic.
		now := time.Now()
		currentMonth := int(now.Month())

		requestedMonthIndex := -1
		months := []string{"january", "february", "march", "april", "may", "june", "july", "august", "september", "october", "november", "december"}
		for i, m := range months {
			if strings.ToLower(month) == m {
				requestedMonthIndex = i + 1
				break
			}
		}

		if requestedMonthIndex != -1 && requestedMonthIndex < currentMonth {
			year = strconv.Itoa(now.Year() + 1)
		}
	}

	artists, err := s.artistRepo.GetActive(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get artists: %w", err)
	}

	artistObjectMap := make(map[string]*model.Artist)
	for i := range artists {
		artistObjectMap[strings.ToLower(artists[i].Name)] = &artists[i]
	}

	monthsList := []string{month}
	links, err := s.scraper.FetchMonthlyLinks(ctx, monthsList, year)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch monthly links for %s-%s: %w", month, year, err)
	}

	if len(links) == 0 {
		s.logger.Warn("No links found for month", zap.String("month", month))
		return 0, nil
	}

	url := links[0]
	s.logger.Info("Found monthly page URL", zap.String("month", month), zap.String("url", url))

	scrapedReleases, err := s.scraper.ParseKProfilesMonthlyPage(ctx, url, month, year)
	if err != nil {
		return 0, fmt.Errorf("failed to parse monthly page: %w", err)
	}

	s.logger.Info("Parsed releases from scraper", zap.Int("count", len(scrapedReleases)))

	savedCount := 0
	for _, scrapedRelease := range scrapedReleases {
		artist, exists := artistObjectMap[strings.ToLower(scrapedRelease.Artist)]
		if !exists {
			continue
		}

		release := &model.Release{
			ArtistID:   artist.ArtistID,
			Title:      scrapedRelease.Title,
			TitleTrack: scrapedRelease.TitleTrack,
			AlbumName:  scrapedRelease.AlbumName,
			MV:         scrapedRelease.MV,
			Spotify:    scrapedRelease.Spotify,
			SourceURL:  scrapedRelease.SourceURL,
			Date:       scrapedRelease.Date,
			IsActive:   true,
		}

		err = s.Upsert(ctx, release)
		if err != nil {
			s.logger.Warn("Failed to save release",
				zap.String("artist", scrapedRelease.Artist),
				zap.Error(err))
			continue
		}

		savedCount++
	}

	s.logger.Info("Completed parsing releases",
		zap.String("month", month),
		zap.Int("parsed", len(scrapedReleases)),
		zap.Int("saved", savedCount))

	return savedCount, nil
}

// GetByArtist searches for and formats releases by an artist's name.
func (s *ReleaseService) GetByArtist(ctx context.Context, artistName string) (string, error) {
	releases, err := s.repo.GetByArtist(ctx, artistName)
	if err != nil {
		return "", fmt.Errorf("failed to get releases for artist %s: %w", artistName, err)
	}

	s.logger.Info("Search results for artist",
		zap.String("artist", artistName),
		zap.Int("count", len(releases)))

	var result strings.Builder
	result.WriteString(fmt.Sprintf("🎵 Artist releases for %s:\n\n", artistName))

	if len(releases) == 0 {
		result.WriteString("No releases found")
		return result.String(), nil
	}

	for _, release := range releases {
		line := FormatReleaseForTelegram(&release)
		result.WriteString(line + "\n")
	}

	return result.String(), nil
}

func monthToInt(month string) int {
	monthMap := map[string]int{
		"january": 1, "february": 2, "march": 3,
		"april": 4, "may": 5, "june": 6,
		"july": 7, "august": 8, "september": 9,
		"october": 10, "november": 11, "december": 12,
	}
	return monthMap[strings.ToLower(month)]
}

// GetTotalReleaseCount returns the total number of release records in the database.
func (s *ReleaseService) GetTotalReleaseCount(ctx context.Context) (int, error) {
	return s.repo.GetTotalCount(ctx)
}
