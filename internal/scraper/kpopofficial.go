package scraper

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// ParseKProfilesMonthlyPage extracts releases for a given month. It prefers
// the WordPress REST API (a handful of JSON requests), and falls back to a
// headless-browser crawl of the monthly schedule page when the API is
// unavailable.
func (f *fetcherImpl) ParseKProfilesMonthlyPage(ctx context.Context, pageURL, month, year string) iter.Seq2[Release, error] {
	return func(yield func(Release, error) bool) {
		if f.parseMonthlyViaREST(ctx, month, year, yield) {
			return
		}

		f.logger.Warn("REST parse failed, falling back to browser-based crawl",
			zap.String("month", month), zap.String("year", year))
		f.logger.Info("Starting browser-based parse", zap.String("url", pageURL))

		// Quick 404 check
		status, err := f.httpClient.CheckStatus(ctx, pageURL)
		if err == nil && status == 404 {
			f.logger.Warn("month page 404, skipping", zap.String("url", pageURL))
			yield(Release{}, fmt.Errorf("page not found: 404"))
			return
		}

		// Browser setup
		timeoutCtx, cancelTimeout := context.WithTimeout(ctx, 10*time.Minute)
		defer cancelTimeout()

		opts := append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.NoSandbox,
			chromedp.DisableGPU,
			chromedp.Flag("disable-dev-shm-usage", true),
			chromedp.Flag("remote-debugging-port", "9222"),
			chromedp.Flag("disable-software-rasterizer", true),
			chromedp.Flag("disable-extensions", true),
			chromedp.Flag("disable-background-networking", true),
			chromedp.Flag("disable-sync", true),
			chromedp.Flag("disable-translate", true),
			chromedp.Flag("metrics-recording-only", true),
			chromedp.Flag("no-first-run", true),
			chromedp.Flag("safebrowsing-disable-auto-update", true),
			chromedp.Flag("blink-settings", "imagesEnabled=false"),
			chromedp.Flag("headless", "new"),
			chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
		)
		allocCtx, cancel := chromedp.NewExecAllocator(timeoutCtx, opts...)
		defer cancel()

		taskCtx, taskCancel := chromedp.NewContext(allocCtx)
		defer taskCancel()

		var finalHTML string
		err = chromedp.Run(taskCtx,
			chromedp.EmulateViewport(1920, 1080),
			chromedp.Navigate(pageURL),
			chromedp.WaitVisible(`body`, chromedp.ByQuery),
			// Kill alerts/dialogs that might hang the browser
			chromedp.Evaluate(`window.alert = window.confirm = window.prompt = function() {};`, nil),
			chromedp.WaitReady(`.gspbgrid_item_link, .wp-block-greenshift-blocks-querygrid`, chromedp.ByQuery),
			chromedp.Sleep(time.Duration(f.config.RequestDelay)*2),

			// Switch to correct month tab
			chromedp.ActionFunc(func(ctx context.Context) error {
				f.logger.Info("selecting release tab", zap.String("month", month))
				var result struct {
					Success bool   `json:"success"`
					Match   string `json:"match"`
				}
				err := chromedp.Evaluate(fmt.Sprintf(`
				(function() {
					const m = "%s".toLowerCase();
					const y = "%s";
					const tabs = Array.from(document.querySelectorAll('.t-btn, [role="tab"], .wp-block-greenshift-blocks-tabs__header-item'));
					
					const queries = ['all ' + m + ' comebacks', 'all ' + m, m + ' comebacks', m + ' ' + y, m];
					for (const q of queries) {
						const el = tabs.find(t => t.textContent.toLowerCase().includes(q));
						if (el) {
							el.scrollIntoView({block: 'center'});
							el.click();
							return { success: true, match: q };
						}
					}
					return { success: false };
				})()
			`, month, year), &result).Do(ctx)

				if err != nil {
					return fmt.Errorf("tab selection failed: %w", err)
				}
				if result.Success {
					f.logger.Info("tab switched", zap.String("pattern", result.Match))
				} else {
					f.logger.Warn("could not find specific month tab, staying on default")
				}
				return nil
			}),

			chromedp.Sleep(3*time.Second),
			chromedp.Evaluate(`window.scrollBy(0, 400);`, nil),

			// Load more content
			chromedp.ActionFunc(func(ctx context.Context) error {
				f.logger.Info("loading more content...")
				var lastCount int
				var stagnant int

				for i := 0; i < 40; i++ {
					var res struct {
						Found bool `json:"found"`
						Count int  `json:"count"`
					}

					err := chromedp.Evaluate(`
					(function() {
						const items = document.querySelectorAll('.gspbgrid_item_link').length;
						const btn = Array.from(document.querySelectorAll('span, button, .gspb-loadmore-btn'))
							.find(b => {
								const r = b.getBoundingClientRect();
								return r.width > 0 && r.height > 0 && b.textContent.toLowerCase().includes('show more');
							});
						
						if (btn) {
							btn.scrollIntoView({block: 'center'});
							btn.click();
							return { found: true, count: items };
						}
						return { found: false, count: items };
					})()
				`, &res).Do(ctx)

					if err != nil {
						f.logger.Error("pagination eval error", zap.Error(err))
						break
					}

					if i%2 == 0 {
						f.logger.Info("parsing batch", zap.Int("i", i), zap.Int("items", res.Count))
					}

					if !res.Found {
						break
					}

					if res.Count > 0 && res.Count == lastCount {
						stagnant++
						if stagnant >= 6 {
							f.logger.Warn("pagination stagnant, breaking", zap.Int("count", res.Count))
							break
						}
					} else {
						stagnant = 0
						lastCount = res.Count
					}

					_ = chromedp.Sleep(3 * time.Second).Do(ctx)
					_ = chromedp.Evaluate(`window.scrollBy(0, 300);`, nil).Do(ctx)
				}
				return nil
			}),
			chromedp.OuterHTML(`html`, &finalHTML),
		)

		var doc *goquery.Document
		if err != nil {
			f.logger.Warn("Chromedp failed, falling back to static HTML parsing", zap.Error(err))
			staticDoc, staticErr := f.httpClient.GetHTML(ctx, pageURL)
			if staticErr != nil {
				yield(Release{}, fmt.Errorf("chromedp failed: %w; static fallback failed: %w", err, staticErr))
				return
			}
			doc = staticDoc
		} else {
			parsedDoc, parseErr := goquery.NewDocumentFromReader(strings.NewReader(finalHTML))
			if parseErr != nil {
				yield(Release{}, parseErr)
				return
			}
			doc = parsedDoc
		}

		var links []string
		doc.Find("a").Each(func(i int, s *goquery.Selection) {
			href, _ := s.Attr("href")
			if strings.Contains(href, "/album/") {
				href = cleanAlbumURL(href)
				if strings.HasPrefix(href, "/") {
					href = "https://kpopofficial.com" + href
				}
				links = append(links, href)
			}
		})

		links = uniqueStrings(links)
		f.logger.Info("Discovered release links", zap.Int("count", len(links)))

		// Sub-crawl logic
		type queueItem struct {
			url   string
			depth int
		}

		type batchResult struct {
			releases []*Release
			links    []string
			depth    int
		}

		queue := make([]queueItem, 0, len(links))
		visited := make(map[string]bool)
		for _, u := range links {
			normU := normalizeURL(u)
			if !visited[normU] {
				visited[normU] = true
				queue = append(queue, queueItem{url: u, depth: 1})
			}
		}

		seenReleases := make(map[string]bool)
		totalProcessed := 0
		const maxLinks = 200

		for len(queue) > 0 && totalProcessed < maxLinks {
			currentBatch := queue
			queue = []queueItem{}
			g, batchCtx := errgroup.WithContext(ctx)
			const maxConcurrency = 3
			sem := make(chan struct{}, maxConcurrency)

			var batchMtx sync.Mutex
			var batchResults []batchResult
			var batchErrors []error

			for _, item := range currentBatch {
				it := item
				g.Go(func() error {
					sem <- struct{}{}
					defer func() { <-sem }()

					delay := f.config.RequestDelay
					if delay == 0 {
						delay = 200 * time.Millisecond
					}
					time.Sleep(delay)

					pageReleases, discoveredLinks, err := f.parseEventPage(batchCtx, it.url, it.depth)
					if err != nil {
						batchMtx.Lock()
						batchErrors = append(batchErrors, fmt.Errorf("page %s: %w", it.url, err))
						batchMtx.Unlock()
						return nil
					}

					batchMtx.Lock()
					batchResults = append(batchResults, batchResult{
						releases: pageReleases,
						links:    discoveredLinks,
						depth:    it.depth,
					})
					batchMtx.Unlock()
					return nil
				})
			}

			if err := g.Wait(); err != nil {
				yield(Release{}, err)
				return
			}

			if len(batchErrors) > 0 {
				f.logger.Warn("Errors occurred during batch crawl", zap.Error(errors.Join(batchErrors...)))
			}

			for _, res := range batchResults {
				totalProcessed++
				for _, r := range res.releases {
					if !releaseInMonth(r.Date, month, year) {
						continue
					}
					relKey := fmt.Sprintf("%s|%s|%s|%s", r.Artist, r.Date.Format("2006-01-02"), strings.ToLower(r.AlbumName), strings.ToLower(r.Title))
					if seenReleases[relKey] {
						continue
					}
					seenReleases[relKey] = true

					if !yield(*r, nil) {
						return
					}
				}
				if res.depth > 0 {
					for _, dl := range res.links {
						normDL := normalizeURL(dl)
						if !visited[normDL] {
							visited[normDL] = true
							queue = append(queue, queueItem{url: dl, depth: res.depth - 1})
						}
					}
				}
			}
		}
	}
}

func (f *fetcherImpl) parseEventPage(ctx context.Context, url string, depth int) ([]*Release, []string, error) {
	doc, err := f.httpClient.GetHTML(ctx, url)
	if err != nil {
		return nil, nil, err
	}
	rels, links, err := f.parseEventPageFromDoc(doc, url)
	if depth > 0 {
		return rels, links, err
	}
	return rels, nil, err
}

// parseMonthlyViaREST yields all releases for the requested month using the
// WordPress REST API. It reports whether the REST path handled the request;
// on transport errors the caller should fall back to the legacy crawl.
func (f *fetcherImpl) parseMonthlyViaREST(ctx context.Context, month, year string, yield func(Release, error) bool) bool {
	after, before, err := monthWindow(month, year)
	if err != nil {
		f.logger.Warn("Cannot build REST window", zap.String("month", month), zap.String("year", year), zap.Error(err))
		return false
	}

	posts, err := f.httpClient.FetchAlbumsWindow(ctx, after, before)
	if err != nil {
		f.logger.Warn("REST album fetch failed", zap.Error(err))
		return false
	}
	f.logger.Info("Fetched albums via REST",
		zap.Int("count", len(posts)),
		zap.String("month", month),
		zap.String("year", year))

	seen := make(map[string]bool)
	for _, post := range posts {
		if ctx.Err() != nil {
			return true
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
			if !releaseInMonth(r.Date, month, year) {
				continue
			}
			key := fmt.Sprintf("%s|%s|%s|%s", r.Artist, r.Date.Format("2006-01-02"), strings.ToLower(r.AlbumName), strings.ToLower(r.Title))
			if seen[key] {
				continue
			}
			seen[key] = true

			if !yield(*r, nil) {
				return true
			}
		}
	}
	return true
}

func (f *fetcherImpl) parseEventPageFromDoc(doc *goquery.Document, url string) ([]*Release, []string, error) {
	pageTitle := strings.TrimSpace(doc.Find("h1.entry-title, .post-title, h1").First().Text())
	defaultArtist, defaultAlbum := splitTitle(pageTitle)

	var metaArtist, metaAlbum, metaTrack, albumLink string
	metaArtist = defaultArtist
	metaAlbum = defaultAlbum

	type eventRaw struct{ Name, Date, Link string }
	var events []eventRaw
	var subLinks []string

	// First pass: gather global metadata (Artist, Album, Title Track)
	doc.Find("tr").Each(func(i int, s *goquery.Selection) {
		cells := s.Find("td")
		if cells.Length() < 2 {
			return
		}

		key := strings.ToLower(strings.Join(strings.Fields(cells.Eq(0).Text()), " "))

		// Clone cell to remove buttons/links from text extraction
		valCell := cells.Eq(1).Clone()
		valCell.Find("a").Each(func(idx int, a *goquery.Selection) {
			atxt := strings.ToLower(a.Text())
			if strings.Contains(atxt, "view") || strings.Contains(atxt, "details") ||
				strings.Contains(atxt, "amazon") || strings.Contains(atxt, "shop") {
				a.Remove()
			}
		})
		val := strings.TrimSpace(valCell.Text())

		switch key {
		case "artist":
			metaArtist = val
		case "album":
			metaAlbum = strings.Trim(val, " “\"”'[]")
			cells.Eq(1).Find("a").Each(func(j int, tag *goquery.Selection) {
				href, _ := tag.Attr("href")
				if strings.Contains(href, "/album/") {
					if strings.HasPrefix(href, "/") {
						href = "https://kpopofficial.com" + href
					}
					albumLink = href
				}
			})
		case "title track", "title":
			metaTrack = strings.Trim(val, " “\"”'[]")
		}
	})

	// Second pass: extract events and follow-up links
	doc.Find("tr").Each(func(i int, s *goquery.Selection) {
		cells := s.Find("td")
		if cells.Length() < 2 {
			return
		}

		// Extract all album/detail links from the second cell for deep crawling
		cells.Eq(1).Find("a").Each(func(j int, tag *goquery.Selection) {
			href, _ := tag.Attr("href")
			if strings.Contains(href, "/album/") {
				href = cleanAlbumURL(href)
				if strings.HasPrefix(href, "/") {
					href = "https://kpopofficial.com" + href
				}
				subLinks = append(subLinks, href)
			}
		})

		// Remove buttons/links from labels
		keyCell := cells.Eq(0).Clone()
		keyCell.Find("a").Each(func(idx int, a *goquery.Selection) {
			atxt := strings.ToLower(a.Text())
			if strings.Contains(atxt, "view") || strings.Contains(atxt, "details") {
				a.Remove()
			}
		})
		key := strings.Join(strings.Fields(keyCell.Text()), " ")
		lowKey := strings.ToLower(key)
		val := strings.TrimSpace(cells.Eq(1).Text())

		// Skip metadata/utility rows
		if lowKey == "artist" || lowKey == "album" || strings.Contains(lowKey, "buy") ||
			strings.Contains(lowKey, "source") || strings.Contains(lowKey, "tracklist") ||
			lowKey == "price" {
			return
		}

		dateStr := findDateInString(val)
		if dateStr != "" {
			title := key
			isMain := strings.Contains(lowKey, "release date") ||
				strings.Contains(lowKey, "album release") ||
				lowKey == "offline release"

			if isMain {
				if albumLink != "" && normalizeURL(albumLink) != normalizeURL(url) {
					return
				}
				title = metaAlbum
			}

			var evtLink string
			cells.Eq(1).Find("a").Each(func(j int, tag *goquery.Selection) {
				href, _ := tag.Attr("href")
				if strings.Contains(href, "/album/") {
					if strings.HasPrefix(href, "/") {
						href = "https://kpopofficial.com" + href
					}
					evtLink = href
				}
			})

			if evtLink == "" || normalizeURL(evtLink) == normalizeURL(url) {
				events = append(events, eventRaw{Name: title, Date: dateStr, Link: evtLink})
			}

			evtLink = cleanAlbumURL(evtLink)
			if evtLink != "" && normalizeURL(evtLink) != normalizeURL(url) {
				subLinks = append(subLinks, evtLink)
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
		releases = append(releases, &Release{
			Artist:     cleanArtistName(metaArtist),
			AlbumName:  metaAlbum,
			Title:      ev.Name,
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

	// Also discover album links from content bodies (discography lists, related album cards)
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

// releaseInMonth reports whether the release date falls into the requested
// month/year. Sub-crawling discovers pages from adjacent months, so out-of-month
// releases are skipped instead of being yielded and stored.
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

// cleanAlbumURL strips query strings and fragments such as "?share=facebook",
// which are share-link variants of the same page and always fail to load.
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
	re := regexp.MustCompile(`(?i)(January|February|March|April|May|June|July|August|September|October|November|December)\s+(\d{1,2}),?\s+(\d{4})`)
	m := re.FindStringSubmatch(t)
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
