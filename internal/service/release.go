package service

import (
	"context"
	"fmt"
	"gemfactory/internal/model"
	"gemfactory/internal/scraper"
	"gemfactory/internal/storage"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/uptrace/bun"
	"go.uber.org/zap"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type ReleaseService struct {
	repo       model.ReleaseRepository
	artistRepo model.ArtistRepository
	scraper    scraper.Fetcher
	logger     *zap.Logger
	mu         sync.Mutex
}

func NewReleaseService(db *bun.DB, scraper scraper.Fetcher, logger *zap.Logger) *ReleaseService {
	return &ReleaseService{
		repo:       storage.NewReleaseRepository(db, logger),
		artistRepo: storage.NewArtistRepository(db, logger),
		scraper:    scraper,
		logger:     logger,
	}
}

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

	if strings.ToLower(month) == "january" && time.Now().Month() == time.December {
		nextYear := time.Now().AddDate(1, 0, 0).Year()
		if year == 0 || year < nextYear {
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
		return "", fmt.Errorf("failed to get releases for %s: %w", month, err)
	}

	var releases []model.Release
	for _, release := range allReleases {
		if gender == "" || (release.Artist != nil && strings.ToLower(string(release.Artist.Gender)) == gender) {
			releases = append(releases, release)
		}
	}

	var result strings.Builder
	caser := cases.Title(language.English)
	if year > 0 {
		fmt.Fprintf(&result, "Releases for %s %d:\n\n", caser.String(month), year)
	} else {
		fmt.Fprintf(&result, "Releases for %s:\n\n", caser.String(month))
	}

	if len(releases) == 0 {
		result.WriteString("No releases found")
		return result.String(), nil
	}

	// Deduplicate releases for the same artist on the same date with matching album/title
	type releaseKey struct {
		artist string
		date   string
		main   string
	}
	dedupMap := make(map[releaseKey]*model.Release)
	var orderedKeys []releaseKey

	for i := range releases {
		rel := &releases[i]
		var aName string
		if rel.Artist != nil {
			aName = strings.ToLower(rel.Artist.Name.String())
		}
		mainEv := cleanReleaseString(rel.AlbumName.String())
		if mainEv == "" {
			mainEv = cleanReleaseString(rel.Title.String())
		}
		key := releaseKey{
			artist: aName,
			date:   rel.Date.Format("2006-01-02"),
			main:   strings.ToLower(mainEv),
		}

		if existing, ok := dedupMap[key]; !ok {
			dedupMap[key] = rel
			orderedKeys = append(orderedKeys, key)
		} else {
			// Merge fields: prefer the entry with non-empty MV, Spotify, Track, etc.
			if (existing.MV.String() == "" || existing.MV.String() == "N/A") && rel.MV.String() != "" && rel.MV.String() != "N/A" {
				existing.MV = rel.MV
			}
			if (existing.Spotify.String() == "" || existing.Spotify.String() == "N/A") && rel.Spotify.String() != "" && rel.Spotify.String() != "N/A" {
				existing.Spotify = rel.Spotify
			}
			if (existing.TitleTrack.String() == "" || existing.TitleTrack.String() == "N/A") && rel.TitleTrack.String() != "" && rel.TitleTrack.String() != "N/A" {
				existing.TitleTrack = rel.TitleTrack
			}
			if strings.Contains(rel.SourceURL.String(), "/album/") && !strings.Contains(existing.SourceURL.String(), "/album/") {
				existing.SourceURL = rel.SourceURL
			}
		}
	}

	for _, k := range orderedKeys {
		line := FormatReleaseForTelegram(dedupMap[k])
		result.WriteString(line + "\n")
	}

	return result.String(), nil
}

func (s *ReleaseService) Upsert(ctx context.Context, release *model.Release) error {
	if err := s.validateRelease(release); err != nil {
		return err
	}

	release.Title = model.NewUniqueString(CleanReleaseTitle(release.Title.String()))
	release.AlbumName = model.NewUniqueString(CleanReleaseTitle(release.AlbumName.String()))
	release.TitleTrack = model.NewUniqueString(CleanReleaseTitle(release.TitleTrack.String()))

	genericTitles := []string{
		"youtube", "official audio", "music video", "mv release",
		"jyp entertainment official youtube", "official youtube",
		"mv", "audio", "video",
	}

	titleLower := strings.ToLower(release.TitleTrack.String())
	for _, generic := range genericTitles {
		if titleLower == generic || strings.Contains(titleLower, generic) {
			release.TitleTrack = model.NewUniqueString("")
			break
		}
	}

	release.MV = model.NewUniqueString(CleanLink(release.MV.String()))
	release.Spotify = model.NewUniqueString(CleanLink(release.Spotify.String()))

	var existingRelease *model.Release
	var err error

	if release.SourceURL.String() != "" {
		existingRelease, err = s.repo.GetByArtistDateAndSource(ctx, release.ArtistID, release.Date, release.SourceURL.String())
	}

	if err == nil && existingRelease == nil {
		if release.TitleTrack.String() != "" && release.TitleTrack.String() != "N/A" {
			existingRelease, err = s.repo.GetByArtistDateAndTrack(ctx, release.ArtistID, release.Date, release.TitleTrack.String())
		}
	}

	if err == nil && existingRelease == nil {
		artistReleases, aErr := s.repo.GetByArtistID(ctx, release.ArtistID)
		if aErr == nil {
			for i := range artistReleases {
				r := &artistReleases[i]
				if r.Date.Equal(release.Date) {
					if strings.EqualFold(r.AlbumName.String(), release.AlbumName.String()) ||
						strings.EqualFold(r.Title.String(), release.Title.String()) ||
						strings.EqualFold(r.TitleTrack.String(), release.TitleTrack.String()) ||
						r.AlbumName.String() == "" || release.AlbumName.String() == "" {
						existingRelease = r
						break
					}
				}
			}
		}
	}

	if err != nil {
		return fmt.Errorf("failed to check for existing release: %w", err)
	}

	if existingRelease != nil {
		if release.Title.String() != "" && release.Title.String() != "N/A" {
			existingRelease.Title = release.Title
		}
		if release.AlbumName.String() != "" && release.AlbumName.String() != "N/A" {
			existingRelease.AlbumName = release.AlbumName
		}
		if release.TitleTrack.String() != "" && release.TitleTrack.String() != "N/A" {
			existingRelease.TitleTrack = release.TitleTrack
		}
		if release.MV.String() != "" && release.MV.String() != "N/A" {
			existingRelease.MV = release.MV
		}
		if release.Spotify.String() != "" && release.Spotify.String() != "N/A" {
			existingRelease.Spotify = release.Spotify
		}
		if release.SourceURL.String() != "" {
			existingRelease.SourceURL = release.SourceURL
		}
		existingRelease.UpdatedAt = time.Now()

		if err := s.repo.Update(ctx, existingRelease); err != nil {
			return err
		}

		artistReleases, aErr := s.repo.GetByArtistID(ctx, release.ArtistID)
		if aErr == nil {
			for i := range artistReleases {
				r := &artistReleases[i]
				if r.ReleaseID != existingRelease.ReleaseID && r.Date.Equal(existingRelease.Date) {
					if strings.EqualFold(r.AlbumName.String(), existingRelease.AlbumName.String()) ||
						strings.EqualFold(r.Title.String(), existingRelease.Title.String()) ||
						r.AlbumName.String() == "" {
						_ = s.repo.Delete(ctx, r.ReleaseID)
					}
				}
			}
		}

		return nil
	}

	return s.repo.Create(ctx, release)
}

func (s *ReleaseService) validateRelease(release *model.Release) error {
	if release == nil {
		return fmt.Errorf("release cannot be nil")
	}
	if release.ArtistID <= 0 {
		return fmt.Errorf("artist_id is required")
	}
	if strings.TrimSpace(release.Title.String()) == "" {
		return fmt.Errorf("title is required")
	}
	if release.Date.IsZero() {
		return fmt.Errorf("date is required")
	}
	return nil
}

func (s *ReleaseService) ParseReleasesForMonth(ctx context.Context, monthName string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

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

	activeArtists, err := s.artistRepo.GetActive(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get artists: %w", err)
	}

	artistObjectMap := make(map[string]*model.Artist)
	for i := range activeArtists {
		artistObjectMap[strings.ToLower(activeArtists[i].Name.String())] = &activeArtists[i]
	}

	savedCount := 0
	for scrapedRelease, err := range s.scraper.ParseMonth(ctx, month, year) {
		if err != nil {
			return savedCount, fmt.Errorf("failed to parse monthly page: %w", err)
		}

		artist := s.findArtist(scrapedRelease.Artist, artistObjectMap)
		if artist == nil {
			s.logger.Debug("Scraped release artist not matched in active artists",
				zap.String("artist", scrapedRelease.Artist),
				zap.String("title", scrapedRelease.Title),
				zap.String("date", scrapedRelease.Date.Format("2006-01-02")))
			continue
		}

		release := &model.Release{
			ArtistID:      artist.ArtistID,
			DisplayArtist: model.NewUniqueString(scrapedRelease.Artist),
			Title:         model.NewUniqueString(scrapedRelease.Title),
			TitleTrack:    model.NewUniqueString(scrapedRelease.TitleTrack),
			AlbumName:     model.NewUniqueString(scrapedRelease.AlbumName),
			MV:            model.NewUniqueString(scrapedRelease.MV),
			Spotify:       model.NewUniqueString(scrapedRelease.Spotify),
			SourceURL:     model.NewUniqueString(scrapedRelease.SourceURL),
			Date:          scrapedRelease.Date,
			IsActive:      true,
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

	s.logger.Info("Completed parsing releases for month",
		zap.String("month", month),
		zap.String("year", year),
		zap.Int("saved", savedCount))

	return savedCount, nil
}

func (s *ReleaseService) ParseReleasesForYear(ctx context.Context, year string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.logger.Info("Starting to parse releases for year", zap.String("year", year))

	activeArtists, err := s.artistRepo.GetActive(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get active artists: %w", err)
	}

	artistObjectMap := make(map[string]*model.Artist)
	for i := range activeArtists {
		artistObjectMap[strings.ToLower(activeArtists[i].Name.String())] = &activeArtists[i]
	}

	savedCount := 0
	for scrapedRelease, err := range s.scraper.ParseYear(ctx, year) {
		if err != nil {
			return savedCount, fmt.Errorf("failed to parse year %s: %w", year, err)
		}

		artist := s.findArtist(scrapedRelease.Artist, artistObjectMap)
		if artist == nil {
			s.logger.Debug("Scraped release artist not matched in active artists",
				zap.String("artist", scrapedRelease.Artist),
				zap.String("title", scrapedRelease.Title),
				zap.String("date", scrapedRelease.Date.Format("2006-01-02")))
			continue
		}

		release := &model.Release{
			ArtistID:      artist.ArtistID,
			DisplayArtist: model.NewUniqueString(scrapedRelease.Artist),
			Title:         model.NewUniqueString(scrapedRelease.Title),
			TitleTrack:    model.NewUniqueString(scrapedRelease.TitleTrack),
			AlbumName:     model.NewUniqueString(scrapedRelease.AlbumName),
			MV:            model.NewUniqueString(scrapedRelease.MV),
			Spotify:       model.NewUniqueString(scrapedRelease.Spotify),
			SourceURL:     model.NewUniqueString(scrapedRelease.SourceURL),
			Date:          scrapedRelease.Date,
			IsActive:      true,
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

	s.logger.Info("Completed parsing releases for year",
		zap.String("year", year),
		zap.Int("saved", savedCount))

	return savedCount, nil
}

func (s *ReleaseService) GetByArtist(ctx context.Context, artistName string) (string, error) {
	releases, err := s.repo.GetByArtist(ctx, artistName)
	if err != nil {
		return "", fmt.Errorf("failed to get releases for artist %s: %w", artistName, err)
	}

	var result strings.Builder
	fmt.Fprintf(&result, "Artist releases for %s:\n\n", artistName)

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

var staticAliases = map[string]string{
	"tomorrow x together": "txt",
}

var collabSplitter = regexp.MustCompile(`(?i)\s+(?:x|feat\.?|ft\.?|with|&)\s+|[,/]`)

func normalizeName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r >= 0x0400 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (s *ReleaseService) findArtist(scrapedName string, artistMap map[string]*model.Artist) *model.Artist {
	raw := strings.ToLower(strings.TrimSpace(scrapedName))
	if raw == "" {
		return nil
	}

	normMap := make(map[string]*model.Artist, len(artistMap))
	for name, artist := range artistMap {
		normMap[normalizeName(name)] = artist
	}

	lookup := func(val string) *model.Artist {
		val = strings.TrimSpace(val)
		if val == "" {
			return nil
		}
		if alias, ok := staticAliases[val]; ok {
			val = alias
		}
		return normMap[normalizeName(val)]
	}

	if a := lookup(raw); a != nil {
		return a
	}

	base := strings.TrimSpace(strings.Split(raw, "(")[0])
	base = strings.TrimSpace(strings.Split(base, " -")[0])
	if a := lookup(base); a != nil {
		return a
	}

	if idx := strings.Index(raw, "("); idx != -1 {
		inside := strings.Trim(raw[idx+1:], ") ")
		if a := lookup(inside); a != nil {
			return a
		}
	}

	candidates := collabSplitter.Split(raw, -1)
	if len(candidates) > 1 {
		for _, cand := range candidates {
			cand = strings.TrimSpace(cand)
			if a := lookup(cand); a != nil {
				return a
			}
			candBase := strings.TrimSpace(strings.Split(cand, "(")[0])
			candBase = strings.TrimSpace(strings.Split(candBase, " -")[0])
			if a := lookup(candBase); a != nil {
				return a
			}
			if idx := strings.Index(cand, "("); idx != -1 {
				inside := strings.Trim(cand[idx+1:], ") ")
				if a := lookup(inside); a != nil {
					return a
				}
			}
		}
	}

	return nil
}

func (s *ReleaseService) GetTotalReleaseCount(ctx context.Context) (int, error) {
	return s.repo.GetTotalCount(ctx)
}
