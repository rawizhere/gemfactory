package service

import (
	"context"
	"fmt"
	"gemfactory/internal/model"
	"gemfactory/internal/scraper"
	"gemfactory/internal/storage"
	"iter"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/uptrace/bun"
	"go.uber.org/zap"
)

type ReleaseService struct {
	repo       model.ReleaseRepository
	artistRepo model.ArtistRepository
	scraper    *scraper.Fetcher
	logger     *zap.Logger
	mu         sync.Mutex
	lastParse  atomic.Pointer[ParseResult]
}

// ParseResult is the outcome of the latest scraper run, shown in the web panel header.
type ParseResult struct {
	At  time.Time
	Err string // empty on success
}

// LastParse returns the latest scraper run since startup, or nil before the first one.
func (s *ReleaseService) LastParse() *ParseResult {
	return s.lastParse.Load()
}

func (s *ReleaseService) recordParse(n int, err error) (int, error) {
	res := &ParseResult{At: time.Now()}
	if err != nil {
		res.Err = err.Error()
	}
	s.lastParse.Store(res)
	return n, err
}

func NewReleaseService(db *bun.DB, scraper *scraper.Fetcher, logger *zap.Logger) *ReleaseService {
	return &ReleaseService{
		repo:       storage.NewReleaseRepository(db),
		artistRepo: storage.NewArtistRepository(db),
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

	startDate := time.Date(year, time.Month(model.MonthNumber(month)), 1, 0, 0, 0, 0, time.UTC)
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
	monthName := time.Month(model.MonthNumber(month)).String()
	if year > 0 {
		fmt.Fprintf(&result, "Releases for %s %d:\n\n", monthName, year)
	} else {
		fmt.Fprintf(&result, "Releases for %s:\n\n", monthName)
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
			aName = strings.ToLower(rel.Artist.Name)
		}
		mainEv := cleanReleaseString(rel.AlbumName)
		if mainEv == "" {
			mainEv = cleanReleaseString(rel.Title)
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
			if (existing.MV == "" || existing.MV == "N/A") && rel.MV != "" && rel.MV != "N/A" {
				existing.MV = rel.MV
			}
			if (existing.Spotify == "" || existing.Spotify == "N/A") && rel.Spotify != "" && rel.Spotify != "N/A" {
				existing.Spotify = rel.Spotify
			}
			if (existing.TitleTrack == "" || existing.TitleTrack == "N/A") && rel.TitleTrack != "" && rel.TitleTrack != "N/A" {
				existing.TitleTrack = rel.TitleTrack
			}
			if strings.Contains(rel.SourceURL, "/album/") && !strings.Contains(existing.SourceURL, "/album/") {
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

	release.Title = CleanReleaseTitle(release.Title)
	release.AlbumName = CleanReleaseTitle(release.AlbumName)
	release.TitleTrack = CleanReleaseTitle(release.TitleTrack)

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

	release.MV = CleanLink(release.MV)
	release.Spotify = CleanLink(release.Spotify)

	var existingRelease *model.Release
	var err error

	if release.SourceURL != "" {
		existingRelease, err = s.repo.GetByArtistDateAndSource(ctx, release.ArtistID, release.Date, release.SourceURL)
	}

	if err == nil && existingRelease == nil {
		if release.TitleTrack != "" && release.TitleTrack != "N/A" {
			existingRelease, err = s.repo.GetByArtistDateAndTrack(ctx, release.ArtistID, release.Date, release.TitleTrack)
		}
	}

	if err == nil && existingRelease == nil {
		artistReleases, aErr := s.repo.GetByArtistID(ctx, release.ArtistID)
		if aErr == nil {
			for i := range artistReleases {
				r := &artistReleases[i]
				if r.Date.Equal(release.Date) {
					if strings.EqualFold(r.AlbumName, release.AlbumName) ||
						strings.EqualFold(r.Title, release.Title) ||
						strings.EqualFold(r.TitleTrack, release.TitleTrack) ||
						r.AlbumName == "" || release.AlbumName == "" {
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

	// Skip placeholder releases: the site links pre-release posts to the parent album,
	// so a shorter album name for the same date adds nothing to the fuller row.
	artistReleases, aErr := s.repo.GetByArtistID(ctx, release.ArtistID)
	if aErr == nil {
		for i := range artistReleases {
			r := &artistReleases[i]
			album := strings.ToLower(strings.TrimSpace(release.AlbumName))
			if r.Date.Equal(release.Date) && album != "" && len(album) < len(r.AlbumName) &&
				strings.HasPrefix(strings.ToLower(strings.TrimSpace(r.AlbumName)), album) {
				return nil
			}
		}
	}

	if existingRelease != nil {
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

		// Merge and drop same-date duplicates, keeping the richer row.
		artistReleases, aErr := s.repo.GetByArtistID(ctx, release.ArtistID)
		if aErr == nil {
			for i := range artistReleases {
				r := &artistReleases[i]
				if r.ReleaseID == existingRelease.ReleaseID || !r.Date.Equal(existingRelease.Date) {
					continue
				}
				if strings.EqualFold(r.AlbumName, existingRelease.AlbumName) ||
					strings.EqualFold(r.Title, existingRelease.Title) ||
					r.AlbumName == "" {
					if existingRelease.TitleTrack == "" && r.TitleTrack != "" {
						existingRelease.TitleTrack = r.TitleTrack
					}
					if existingRelease.MV == "" && r.MV != "" {
						existingRelease.MV = r.MV
					}
					if existingRelease.Spotify == "" && r.Spotify != "" {
						existingRelease.Spotify = r.Spotify
					}
					_ = s.repo.Delete(ctx, r.ReleaseID)
				}
			}
		}

		if err := s.repo.Update(ctx, existingRelease); err != nil {
			return err
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
	if strings.TrimSpace(release.Title) == "" {
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

	month, year := resolveMonthYear(monthName)
	s.logger.Info("Starting to parse releases", zap.String("month", monthName))
	return s.recordParse(s.parseReleases(ctx, s.scraper.ParseMonth(ctx, month, year), "month",
		"failed to parse monthly page",
		[]zap.Field{zap.String("month", month), zap.String("year", year)}))
}

func (s *ReleaseService) ParseReleasesForYear(ctx context.Context, year string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.logger.Info("Starting to parse releases for year", zap.String("year", year))
	return s.recordParse(s.parseReleases(ctx, s.scraper.ParseYear(ctx, year), "year",
		"failed to parse year "+year,
		[]zap.Field{zap.String("year", year)}))
}

// resolveMonthYear splits a "month-year" input and shifts to next year for past months without an explicit year.
func resolveMonthYear(monthName string) (month, year string) {
	if !strings.Contains(monthName, "-") {
		if m := model.MonthNumber(monthName); m > 0 && m < int(time.Now().Month()) {
			return monthName, strconv.Itoa(time.Now().Year() + 1)
		}
		return monthName, time.Now().Format("2006")
	}
	parts := strings.Split(monthName, "-")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return monthName, time.Now().Format("2006")
}

// parseReleases scrapes releases from the given iterator and upserts those with matched active artists.
func (s *ReleaseService) parseReleases(ctx context.Context, releases iter.Seq2[scraper.Release, error], period, scrapeErrPrefix string, endFields []zap.Field) (int, error) {
	activeArtists, err := s.artistRepo.GetActive(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get artists: %w", err)
	}

	artistObjectMap := make(map[string]*model.Artist)
	for i := range activeArtists {
		artistObjectMap[strings.ToLower(activeArtists[i].Name)] = &activeArtists[i]
	}

	savedCount := 0
	for scrapedRelease, err := range releases {
		if err != nil {
			return savedCount, fmt.Errorf("%s: %w", scrapeErrPrefix, err)
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
			DisplayArtist: scrapedRelease.Artist,
			Title:         scrapedRelease.Title,
			TitleTrack:    scrapedRelease.TitleTrack,
			AlbumName:     scrapedRelease.AlbumName,
			MV:            scrapedRelease.MV,
			Spotify:       scrapedRelease.Spotify,
			SourceURL:     scrapedRelease.SourceURL,
			Date:          scrapedRelease.Date,
			IsActive:      true,
		}

		if err := s.Upsert(ctx, release); err != nil {
			s.logger.Warn("Failed to save release",
				zap.String("artist", scrapedRelease.Artist),
				zap.Error(err))
			continue
		}

		savedCount++
	}

	s.logger.Info("Completed parsing releases for "+period, append(endFields, zap.Int("saved", savedCount))...)

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

var staticAliases = map[string]string{
	"tomorrow x together": "txt",
	"ichillin j":          "ichillin'",
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

	// variants tries the name as-is, without a "(...)" / " -" suffix, and the text inside the parens.
	variants := func(name string) *model.Artist {
		if a := lookup(name); a != nil {
			return a
		}
		base := strings.TrimSpace(strings.Split(name, "(")[0])
		base = strings.TrimSpace(strings.Split(base, " -")[0])
		if a := lookup(base); a != nil {
			return a
		}
		if idx := strings.Index(name, "("); idx != -1 {
			return lookup(strings.Trim(name[idx+1:], ") "))
		}
		return nil
	}

	if a := variants(raw); a != nil {
		return a
	}
	if candidates := collabSplitter.Split(raw, -1); len(candidates) > 1 {
		for _, cand := range candidates {
			if a := variants(strings.TrimSpace(cand)); a != nil {
				return a
			}
		}
	}

	return nil
}

func (s *ReleaseService) GetTotalReleaseCount(ctx context.Context) (int, error) {
	return s.repo.GetTotalCount(ctx)
}
