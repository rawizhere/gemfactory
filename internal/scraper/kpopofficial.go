package scraper

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// ParseKProfilesMonthlyPage extracts the monthly release schedule from the kpopofficial website.
// It performs deep crawling of individual event pages to gather Spotify and MV links.
func (f *fetcherImpl) ParseKProfilesMonthlyPage(ctx context.Context, url, month, year string) ([]Release, error) {
	f.logger.Info("Starting to parse kpopofficial page",
		zap.String("url", url),
		zap.String("month", month),
		zap.String("year", year))

	// Get main monthly page.
	var doc *goquery.Document
	err := WithRetry(ctx, f.logger, f.config.RetryConfig, func() error {
		var err error
		doc, err = f.httpClient.GetHTML(ctx, url)
		if err != nil {
			if strings.Contains(err.Error(), "404") {
				return &PermanentError{Err: err}
			}
			return err
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to fetch page: %w", err)
	}

	// Scan for event links within grid items (cards).
	var eventsToVisit []string

	doc.Find(".gspbgrid_item").Each(func(i int, s *goquery.Selection) {
		// Look for the main link in the card.
		href, exists := s.Find("a.gspbgrid_item_link").Attr("href")
		if !exists {
			// Fallback: any link in the card that looks like an album link.
			href, exists = s.Find("a").Attr("href")
		}

		if exists && strings.Contains(href, "/album/") {
			if strings.HasPrefix(href, "/") {
				href = "https://kpopofficial.com" + href
			}
			eventsToVisit = append(eventsToVisit, href)
		}
	})

	// Fallback for pages without grid items (older or different structure).
	if len(eventsToVisit) == 0 {
		doc.Find("a").Each(func(i int, s *goquery.Selection) {
			href, exists := s.Attr("href")
			if !exists {
				return
			}
			if strings.Contains(href, "kpopofficial.com/album/") {
				eventsToVisit = append(eventsToVisit, href)
			}
		})
	}

	// Deduplicate discovered links.
	eventsToVisit = uniqueStrings(eventsToVisit)
	f.logger.Info("Found event links to crawl", zap.Int("count", len(eventsToVisit)))

	var releases []Release
	visited := make(map[string]bool)
	for _, u := range eventsToVisit {
		visited[u] = true
	}

	// Visit each event page to extract details.
	// We use an iterative approach with a queue to support deep crawling discovered links.
	queue := eventsToVisit
	for len(queue) > 0 {
		currentBatch := queue
		queue = []string{} // Reset queue for next depth.

		g, batchCtx := errgroup.WithContext(ctx)
		const maxConcurrency = 5 // Slightly lower concurrency for safety.
		sem := make(chan struct{}, maxConcurrency)
		var releasesMutex sync.Mutex
		var discoveredMutex sync.Mutex

		for _, eventURL := range currentBatch {
			url := eventURL
			g.Go(func() error {
				sem <- struct{}{}
				defer func() { <-sem }()

				select {
				case <-batchCtx.Done():
					return batchCtx.Err()
				default:
				}

				if f.config.RequestDelay > 0 {
					time.Sleep(f.config.RequestDelay)
				}

				pageReleases, discoveredLinks, err := f.parseEventPage(batchCtx, url)
				if err != nil {
					f.logger.Error("Failed to parse event page", zap.String("url", url), zap.Error(err))
					return nil
				}

				if len(pageReleases) > 0 {
					releasesMutex.Lock()
					for _, r := range pageReleases {
						releases = append(releases, *r)
					}
					releasesMutex.Unlock()
				}

				if len(discoveredLinks) > 0 {
					discoveredMutex.Lock()
					for _, dl := range discoveredLinks {
						if !visited[dl] {
							visited[dl] = true
							queue = append(queue, dl)
						}
					}
					discoveredMutex.Unlock()
				}
				return nil
			})
		}

		if err := g.Wait(); err != nil {
			return nil, err
		}

		if len(queue) > 0 {
			f.logger.Info("Deep crawl: discovered more links", zap.Int("count", len(queue)))
		}
	}

	return releases, nil
}

// ScrapedRelease represents the raw data extracted during the scraping process.
type ScrapedRelease struct {
	Artist     string
	AlbumName  string
	Title      string
	TitleTrack string
	Date       time.Time
	MV         string
	Spotify    string
	SourceURL  string
}

// parseEventPage fetches and extracts data from a specific album/event page.
// It returns a list of releases found on the page and any sub-links discovered in tables.
func (f *fetcherImpl) parseEventPage(ctx context.Context, url string) ([]*Release, []string, error) {
	var doc *goquery.Document
	err := WithRetry(ctx, f.logger, f.config.RetryConfig, func() error {
		var err error
		doc, err = f.httpClient.GetHTML(ctx, url)
		return err
	})

	if err != nil {
		return nil, nil, err
	}

	releases, links, err := f.parseEventPageFromDoc(doc, url)
	if err != nil {
		html, _ := doc.Html()
		f.logger.Error("Failed to parse event page content",
			zap.String("url", url),
			zap.String("html_snippet", truncateString(html, 1000)))
		return nil, nil, err
	}
	return releases, links, nil
}

// parseEventPageFromDoc extracts structured release data from an HTML document.
func (f *fetcherImpl) parseEventPageFromDoc(doc *goquery.Document, url string) ([]*Release, []string, error) {
	// Extract Title & Artist from Page Title or Header as fallback/default.
	pageTitle := doc.Find("h1.entry-title").Text()
	if pageTitle == "" {
		pageTitle = doc.Find(".post-title").Text()
	}
	if pageTitle == "" {
		pageTitle = doc.Find("h1").First().Text()
	}
	if pageTitle == "" {
		pageTitle = doc.Find("title").Text()
	}
	pageTitle = strings.TrimSpace(pageTitle)
	defaultArtist, defaultAlbum := splitTitle(pageTitle)

	// Global metadata containers.
	var metaAlbumName = defaultAlbum
	var metaTitleTrack = ""
	var metaArtist = defaultArtist

	// Events found in the table.
	type eventRaw struct {
		Name string
		Date string
		Link string
	}
	var events []eventRaw
	var discoveredSubLinks []string

	// getText extracts cleaned plain text from a selection, handling line breaks and metadata cleanup.
	getText := func(s *goquery.Selection) string {
		html, _ := s.Html()
		// Replace common block tags with space.
		html = strings.ReplaceAll(html, "<br>", " ")
		html = strings.ReplaceAll(html, "<br/>", " ")
		html = strings.ReplaceAll(html, "<br />", " ")

		// Create a temporary doc to extract text cleanly.
		tmpDoc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
		text := tmpDoc.Text()

		// Remove "View Details" or other common button text if stuck to the date.
		text = strings.ReplaceAll(text, "[View Details]", "")

		return strings.TrimSpace(text)
	}

	// First pass: Scan table for metadata AND events.
	doc.Find("tr").Each(func(i int, s *goquery.Selection) {
		cells := s.Find("td")
		if cells.Length() < 2 {
			return
		}

		col1 := getText(cells.Eq(0))
		col2 := getText(cells.Eq(1))

		// Check keys.
		k := strings.ToLower(col1)

		if strings.Contains(k, "artist") && !strings.Contains(k, "feat") {
			if val := col2; val != "" {
				metaArtist = val
			}
			return
		}
		if k == "album" || k == "album title" {
			if val := col2; val != "" {
				re := regexp.MustCompile(`\[.*?\]`)
				val = re.ReplaceAllString(val, "")
				metaAlbumName = strings.TrimSpace(val)
			}
			return
		}

		if k == "title track" || k == "title" {
			if val := col2; val != "" {
				val = strings.Trim(val, " \"”")
				val = strings.ReplaceAll(val, "“", "")
				val = strings.ReplaceAll(val, "”", "")
				val = strings.ReplaceAll(val, "\"", "")
				metaTitleTrack = strings.TrimSpace(val)
			}
			return
		}

		// Check if it looks like an event (has date in col2).
		dateStr := findDateInString(col2)
		if dateStr != "" {
			var evtLink string
			cells.Eq(1).Find("a").Each(func(j int, tag *goquery.Selection) {
				href, exists := tag.Attr("href")
				if exists {
					// Handle different types of links.
					if strings.Contains(tag.Text(), "View Details") || strings.Contains(href, "/album/") {
						if strings.HasPrefix(href, "/") {
							href = "https://kpopofficial.com" + href
						}
						evtLink = href
						discoveredSubLinks = append(discoveredSubLinks, href)
					}
				}
			})

			events = append(events, eventRaw{Name: col1, Date: dateStr, Link: evtLink})
		}
	})

	// Shared links (only used if NOT a sub-link event).
	globalYoutubeLink := findIframeSrc(doc, "youtube.com/embed")
	if globalYoutubeLink == "" {
		globalYoutubeLink = findLinkByDomain(doc, "youtube.com", "youtu.be")
	}
	globalSpotifyLink := findLinkByDomain(doc, "open.spotify.com")

	// Fallback Title Track.
	if metaTitleTrack == "" {
		metaTitleTrack = findLineWithPrefix(doc, "Title Track")
		metaTitleTrack = strings.TrimPrefix(metaTitleTrack, "Title Track")
		metaTitleTrack = strings.TrimPrefix(metaTitleTrack, ":")
		metaTitleTrack = strings.TrimSpace(metaTitleTrack)
		metaTitleTrack = strings.Trim(metaTitleTrack, " \"”")
	}

	var foundReleases []*Release

	// Process events.
	for _, ev := range events {
		title := ev.Name
		isMainAlbumRelease := false

		// Clean title.
		lowerTitle := strings.ToLower(title)
		if strings.Contains(lowerTitle, "album release") || strings.Contains(lowerTitle, "release date") || lowerTitle == "offline release" {
			title = metaAlbumName
			isMainAlbumRelease = true
		}

		var date time.Time
		d, err := parseKProfilesDate(ev.Date)
		if err == nil {
			date = d
		}

		// Prefer sub-page links over global links for specific events.
		mvLink := globalYoutubeLink
		spotifyLink := globalSpotifyLink
		sourceURL := url // Default page URL as fallback.
		if ev.Link != "" {
			mvLink = ""
			spotifyLink = ""
			sourceURL = ev.Link // Specific event page URL.
		}

		r := &Release{
			Artist:    cleanArtistName(metaArtist),
			AlbumName: metaAlbumName,
			Title:     title,
			Date:      date,
			MV:        mvLink,
			Spotify:   spotifyLink,
			SourceURL: sourceURL,
		}

		// Assign Title Track.
		if isMainAlbumRelease {
			r.TitleTrack = metaTitleTrack
		} else {
			safeTitle := strings.ReplaceAll(title, "“", "\"")
			safeTitle = strings.ReplaceAll(safeTitle, "”", "\"")
			if strings.Contains(safeTitle, "\"") {
				parts := strings.Split(safeTitle, "\"")
				if len(parts) >= 2 {
					r.TitleTrack = parts[1]
				}
			}
		}

		foundReleases = append(foundReleases, r)
	}

	// Strategy 2: Fallback.
	if len(foundReleases) == 0 {
		var date time.Time
		ogDesc, exists := doc.Find("meta[property='og:description']").Attr("content")
		if exists {
			dateStr := findDateInString(ogDesc)
			if dateStr != "" {
				date, _ = parseKProfilesDate(dateStr)
			}
		}
		if date.IsZero() {
			dateStr := findDateInContent(doc)
			if dateStr != "" {
				date, _ = parseKProfilesDate(dateStr)
			}
		}

		if !date.IsZero() {
			r := &Release{
				Artist:     cleanArtistName(metaArtist),
				AlbumName:  metaAlbumName,
				Title:      metaAlbumName,
				TitleTrack: metaTitleTrack,
				Date:       date,
				MV:         globalYoutubeLink,
				Spotify:    globalSpotifyLink,
				SourceURL:  url,
			}
			foundReleases = append(foundReleases, r)
		}
	}

	return foundReleases, uniqueStrings(discoveredSubLinks), nil
}

// findIframeSrc extracts the source URL from an iframe containing a specific domain part.
func findIframeSrc(doc *goquery.Document, domainPart string) string {
	var src string
	doc.Find("iframe").EachWithBreak(func(i int, s *goquery.Selection) bool {
		val, exists := s.Attr("src")
		if exists && strings.Contains(val, domainPart) {
			src = val
			// Standardize YouTube embed links to regular watch links.
			if strings.Contains(src, "youtube.com/embed/") {
				id := strings.Split(src, "embed/")[1]
				if idx := strings.Index(id, "?"); idx != -1 {
					id = id[:idx]
				}
				src = "https://www.youtube.com/watch?v=" + id
			}
			return false
		}
		return true
	})

	if isYouTubeChannel(src) {
		return ""
	}

	return src
}

// isYouTubeChannel verifies if a given URL points to a channel profile rather than a video.
func isYouTubeChannel(link string) bool {
	low := strings.ToLower(link)
	return strings.Contains(low, "youtube.com/@") ||
		strings.Contains(low, "youtube.com/channel/") ||
		strings.Contains(low, "youtube.com/user/") ||
		strings.Contains(low, "youtube.com/c/")
}

// uniqueStrings returns a new slice with duplicate strings removed.
func uniqueStrings(input []string) []string {
	keys := make(map[string]bool)
	list := []string{}
	for _, entry := range input {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}

// splitTitle attempts to separate an artist name and album title from a combined string.
func splitTitle(input string) (string, string) {
	// Format "Artist – Album".
	parts := strings.SplitN(input, "–", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	// Try "-".
	parts = strings.SplitN(input, "-", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return strings.TrimSpace(input), ""
}

// findLinkByDomain searches the document for hyperlinks matching specific domain patterns.
func findLinkByDomain(doc *goquery.Document, domains ...string) string {
	var link string
	// Search globally, not just in entry-content, as buttons might be outside.
	doc.Find("a").EachWithBreak(func(i int, s *goquery.Selection) bool {
		href, exists := s.Attr("href")
		if !exists {
			return true
		}
		for _, d := range domains {
			if strings.Contains(href, d) {
				// Special check for YouTube links to avoid channels.
				if strings.Contains(d, "youtube.com") || strings.Contains(d, "youtu.be") {
					if isYouTubeChannel(href) {
						continue
					}
				}
				link = href
				return false // found.
			}
		}
		return true
	})
	return link
}

// findLineWithPrefix searches for a line of text within paragraph or list item elements that starts with a given prefix.
func findLineWithPrefix(doc *goquery.Document, prefix string) string {
	var result string
	doc.Find("p, li").EachWithBreak(func(i int, s *goquery.Selection) bool {
		text := s.Text()
		if strings.Contains(strings.ToLower(text), strings.ToLower(prefix)) {
			lines := strings.Split(text, "\n")
			for _, line := range lines {
				if strings.Contains(strings.ToLower(line), strings.ToLower(prefix)) {
					result = line
					return false
				}
			}
		}
		return true
	})
	return result
}

// findDateInString extracts a date string in "Month DD, YYYY" format from the input text.
func findDateInString(text string) string {
	// Regex for "Month DD, YYYY".
	re := regexp.MustCompile(`(January|February|March|April|May|June|July|August|September|October|November|December)\s+\d{1,2},\s+\d{4}`)
	match := re.FindString(text)
	return match
}

// findDateInContent searches for a date string within the document's entry content.
func findDateInContent(doc *goquery.Document) string {
	// Look for standard date patterns in text.
	content := doc.Find(".entry-content").Text()
	return findDateInString(content)
}

// cleanArtistName removes decorative symbols and emojis from the artist's name.
func cleanArtistName(raw string) string {
	raw = strings.TrimSpace(raw)

	// Remove Unicode emojis and special symbols.
	var cleaned []rune
	for _, r := range raw {
		// Keep normal letters, numbers and basic symbols (like & or -).
		// Ranges:
		// Basic Latin: 0x0020 - 0x007F
		// Latin-1 Supplement: 0x00A0 - 0x00FF (includes accents)
		// Korean Hangul: 0xAC00 - 0xD7AF and 0x1100 - 0x11FF
		// CJK Unified Ideographs: 0x4E00 - 0x9FFF
		// Hiragana/Katakana could be added if needed.

		// Simple allow-list approach for now:
		if r < 0x2000 { // Basic multilingual plane, covering most languages
			cleaned = append(cleaned, r)
		}
	}

	raw = string(cleaned)
	return strings.TrimSpace(raw)
}

// parseKProfilesDate parses date in "January 1, 2026" format.
func parseKProfilesDate(dateStr string) (time.Time, error) {
	return time.Parse("January 2, 2006", dateStr)
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "... [truncated]"
}
