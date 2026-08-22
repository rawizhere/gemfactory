package scraper

import (
	"context"
	"fmt"
	"iter"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"go.uber.org/zap"
)

var dateRegex = regexp.MustCompile(`(?i)(January|February|March|April|May|June|July|August|September|October|November|December)\s+(\d{1,2}),?\s+(\d{4})`)

// ParseMonth yields all releases for the given month using WordPress REST API.
func (f *fetcherImpl) ParseMonth(ctx context.Context, month, year string) iter.Seq2[Release, error] {
	return func(yield func(Release, error) bool) {
		after, before, err := monthWindow(month, year)
		if err != nil {
			yield(Release{}, err)
			return
		}
		f.parseRESTWindow(ctx, after, before, month, year, yield)
	}
}

// ParseYear yields all releases for the entire given year using WordPress REST API.
func (f *fetcherImpl) ParseYear(ctx context.Context, year string) iter.Seq2[Release, error] {
	return func(yield func(Release, error) bool) {
		after, before, err := yearWindow(year)
		if err != nil {
			yield(Release{}, err)
			return
		}
		f.parseRESTWindow(ctx, after, before, "", year, yield)
	}
}

func (f *fetcherImpl) parseRESTWindow(ctx context.Context, after, before time.Time, month, year string, yield func(Release, error) bool) {
	posts, err := f.httpClient.FetchAlbumsWindow(ctx, after, before)
	if err != nil {
		f.logger.Warn("REST album fetch failed", zap.Error(err))
		yield(Release{}, err)
		return
	}
	f.logger.Info("Fetched albums via REST",
		zap.Int("count", len(posts)),
		zap.String("month", month),
		zap.String("year", year))

	seen := make(map[string]bool)
	for _, post := range posts {
		if ctx.Err() != nil {
			return
		}
		if post.Link == "" || post.Content.Rendered == "" {
			continue
		}
		wrapped := "<h1 class=\"entry-title\">" + post.Title.Rendered + "</h1>" +
			"<div class=\"entry-content\">" + post.Content.Rendered + "</div>"
		doc, docErr := goquery.NewDocumentFromReader(strings.NewReader(wrapped))
		if docErr != nil {
			continue
		}

		rels, _, parseErr := f.parseEventPageFromDoc(doc, post.Link)
		if parseErr != nil {
			continue
		}

		for _, r := range rels {
			if month != "" {
				if !releaseInMonth(r.Date, month, year) {
					continue
				}
			} else if year != "" {
				if !releaseInYear(r.Date, year) {
					continue
				}
			}
			key := fmt.Sprintf("%s|%s|%s|%s", r.Artist, r.Date.Format("2006-01-02"), strings.ToLower(r.AlbumName), strings.ToLower(r.Title))
			if seen[key] {
				continue
			}
			seen[key] = true

			if !yield(*r, nil) {
				return
			}
		}
	}
}

func (f *fetcherImpl) parseEventPageFromDoc(doc *goquery.Document, url string) ([]*Release, []string, error) {
	pageTitle := strings.TrimSpace(doc.Find("h1.entry-title, .post-title, h1").First().Text())
	defaultArtist, defaultAlbum := splitTitle(pageTitle)

	var metaArtist, metaAlbum, metaTrack string
	metaArtist = defaultArtist
	metaAlbum = defaultAlbum

	type eventRaw struct {
		Name   string
		Date   string
		IsMain bool
	}
	var events []eventRaw
	var subLinks []string

	// Single-pass extraction over table rows
	doc.Find("tr").Each(func(i int, s *goquery.Selection) {
		cells := s.Find("td")
		if cells.Length() < 2 {
			return
		}

		// Clean key cell
		keyCell := cells.Eq(0).Clone()
		keyCell.Find("a").Each(func(idx int, a *goquery.Selection) {
			atxt := strings.ToLower(a.Text())
			if strings.Contains(atxt, "view") || strings.Contains(atxt, "details") {
				a.Remove()
			}
		})
		key := strings.Join(strings.Fields(keyCell.Text()), " ")
		lowKey := strings.ToLower(key)

		// Clean value cell
		valCell := cells.Eq(1).Clone()
		valCell.Find("a").Each(func(idx int, a *goquery.Selection) {
			atxt := strings.ToLower(a.Text())
			if strings.Contains(atxt, "view") || strings.Contains(atxt, "details") ||
				strings.Contains(atxt, "amazon") || strings.Contains(atxt, "shop") {
				a.Remove()
			}
		})
		val := strings.TrimSpace(valCell.Text())

		// Extract album links for subLinks
		cells.Eq(1).Find("a").Each(func(j int, tag *goquery.Selection) {
			href, _ := tag.Attr("href")
			if strings.Contains(href, "/album/") {
				if strings.HasPrefix(href, "/") {
					href = "https://kpopofficial.com" + href
				}
				subLinks = append(subLinks, cleanAlbumURL(href))
			}
		})

		switch lowKey {
		case "artist":
			metaArtist = val
		case "album":
			metaAlbum = strings.Trim(val, " “\"”'[]")
		case "title track", "title":
			metaTrack = strings.Trim(val, " “\"”'[]")
		default:
			if strings.Contains(lowKey, "buy") || strings.Contains(lowKey, "source") ||
				strings.Contains(lowKey, "tracklist") || lowKey == "price" {
				return
			}
			dateStr := findDateInString(val)
			if dateStr != "" {
				isMain := strings.Contains(lowKey, "release date") ||
					strings.Contains(lowKey, "album release") ||
					lowKey == "offline release"
				events = append(events, eventRaw{
					Name:   key,
					Date:   dateStr,
					IsMain: isMain,
				})
			}
		}
	})

	yt, _ := doc.Find("iframe[src*='youtube']").Attr("src")
	if strings.Contains(yt, "embed/") {
		id := strings.Split(yt, "embed/")[1]
		if idx := strings.Index(id, "?"); idx != -1 {
			id = id[:idx]
		}
		yt = "https://www.youtube.com/watch?v=" + id
	}

	sp, _ := doc.Find("a[href*='open.spotify.com']").Attr("href")
	if sp == "" {
		spIframe, _ := doc.Find("iframe[src*='open.spotify.com']").Attr("src")
		if spIframe != "" {
			sp = strings.Replace(spIframe, "/embed/", "/", 1)
		}
	}
	if sp != "" {
		if idx := strings.Index(sp, "?"); idx != -1 {
			sp = sp[:idx]
		}
	}

	var releases []*Release
	for _, ev := range events {
		d, _ := time.Parse("January 2, 2006", ev.Date)
		title := ev.Name
		if ev.IsMain || title == "" {
			title = metaAlbum
		}
		releases = append(releases, &Release{
			Artist:     cleanArtistName(metaArtist),
			AlbumName:  metaAlbum,
			Title:      title,
			TitleTrack: metaTrack,
			Date:       d,
			MV:         yt,
			Spotify:    sp,
			SourceURL:  url,
		})
	}
	if len(releases) == 0 {
		if ds := findDateInString(doc.Find(".entry-content").Text()); ds != "" {
			d, _ := time.Parse("January 2, 2006", ds)
			releases = append(releases, &Release{
				Artist:     cleanArtistName(metaArtist),
				AlbumName:  metaAlbum,
				Title:      metaAlbum,
				TitleTrack: metaTrack,
				Date:       d,
				MV:         yt,
				Spotify:    sp,
				SourceURL:  url,
			})
		}
	}

	// Also discover album links from content bodies
	doc.Find(".entry-content a, .post a, .post-inner a, .wp-block-post-template a, .gspbgrid_item_link").Each(func(j int, tag *goquery.Selection) {
		href, _ := tag.Attr("href")
		if strings.Contains(href, "/album/") {
			href = cleanAlbumURL(href)
			if strings.HasPrefix(href, "/") {
				href = "https://kpopofficial.com" + href
			}
			if normalizeURL(href) != normalizeURL(url) {
				subLinks = append(subLinks, href)
			}
		}
	})

	return releases, uniqueStrings(subLinks), nil
}

// releaseInMonth reports whether the release date falls into the requested month/year.
func releaseInMonth(d time.Time, month, year string) bool {
	if d.IsZero() {
		return false
	}
	m := monthNumber(month)
	if m == 0 {
		return true
	}
	if int(d.Month()) != m {
		return false
	}
	if y, err := strconv.Atoi(strings.TrimSpace(year)); err == nil && y > 0 {
		return d.Year() == y
	}
	return true
}

// releaseInYear reports whether the release date falls into the requested year.
func releaseInYear(d time.Time, year string) bool {
	if d.IsZero() {
		return false
	}
	if y, err := strconv.Atoi(strings.TrimSpace(year)); err == nil && y > 0 {
		return d.Year() == y
	}
	return true
}

func monthNumber(name string) int {
	months := []string{"january", "february", "march", "april", "may", "june", "july", "august", "september", "october", "november", "december"}
	for i, m := range months {
		if strings.ToLower(strings.TrimSpace(name)) == m {
			return i + 1
		}
	}
	return 0
}

func normalizeURL(u string) string {
	u = cleanAlbumURL(u)
	u = strings.TrimSuffix(u, "/")
	return strings.ToLower(u)
}

// cleanAlbumURL strips query strings and fragments.
func cleanAlbumURL(u string) string {
	if idx := strings.IndexAny(u, "?#"); idx != -1 {
		u = u[:idx]
	}
	return u
}

func uniqueStrings(in []string) []string {
	m := make(map[string]bool)
	var out []string
	for _, s := range in {
		normalized := normalizeURL(s)
		if !m[normalized] {
			m[normalized] = true
			out = append(out, s)
		}
	}
	return out
}

func splitTitle(in string) (string, string) {
	p := strings.SplitN(in, "–", 2)
	if len(p) < 2 {
		p = strings.SplitN(in, "-", 2)
	}
	if len(p) == 2 {
		return strings.TrimSpace(p[0]), strings.TrimSpace(p[1])
	}
	return in, ""
}

func findDateInString(t string) string {
	t = strings.ReplaceAll(t, "\u00a0", " ")
	m := dateRegex.FindStringSubmatch(t)
	if len(m) == 4 {
		month := strings.ToUpper(m[1][:1]) + strings.ToLower(m[1][1:])
		return fmt.Sprintf("%s %s, %s", month, m[2], m[3])
	}
	return ""
}

func cleanArtistName(s string) string {
	var r []rune
	for _, c := range s {
		if c < 0x2000 {
			r = append(r, c)
		}
	}
	return strings.TrimSpace(string(r))
}
