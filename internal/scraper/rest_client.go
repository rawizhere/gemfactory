package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// albumTitle is the title wrapper of a WordPress REST post.
type albumTitle struct {
	Rendered string `json:"rendered"`
}

// albumContent is the content wrapper of a WordPress REST post.
type albumContent struct {
	Rendered string `json:"rendered"`
}

// AlbumPost is a single album page fetched from the WordPress REST API.
type AlbumPost struct {
	Link    string       `json:"link"`
	Title   albumTitle   `json:"title"`
	Content albumContent `json:"content"`
}

const albumsPerPage = 100

// FetchAlbumsWindow retrieves all album posts whose publication date falls
// within [after, before], following REST pagination. It returns fully rendered
// page bodies, so no headless browser or per-page crawling is required.
func (c *HTTPClient) FetchAlbumsWindow(ctx context.Context, after, before time.Time) ([]AlbumPost, error) {
	var all []AlbumPost

	for page := 1; ; page++ {
		q := url.Values{}
		q.Set("per_page", strconv.Itoa(albumsPerPage))
		q.Set("page", strconv.Itoa(page))
		q.Set("after", after.Format(time.RFC3339))
		q.Set("before", before.Format(time.RFC3339))
		q.Set("_fields", "link,title,content")
		apiURL := "https://kpopofficial.com/wp-json/wp/v2/album?" + q.Encode()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create rest request: %w", err)
		}
		req.Header.Set("User-Agent", c.userAgent)
		req.Header.Set("Accept", "application/json")

		resp, err := c.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("rest request failed: %w", err)
		}

		var posts []AlbumPost
		decodeErr := json.NewDecoder(resp.Body).Decode(&posts)
		closeErr := resp.Body.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf("failed to decode rest response: %w", decodeErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("failed to close response body: %w", closeErr)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected rest status code: %d", resp.StatusCode)
		}

		all = append(all, posts...)

		if len(posts) < albumsPerPage {
			break
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}

	return all, nil
}

// monthWindow returns the publication-date window that is likely to contain
// all album pages for releases in the given month: the month itself padded by
// 45 days on each side.
func monthWindow(month, year string) (time.Time, time.Time, error) {
	m := monthNumber(month)
	if m == 0 {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid month: %q", strings.TrimSpace(month))
	}
	y, err := strconv.Atoi(strings.TrimSpace(year))
	if err != nil || y <= 0 {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid year: %q", year)
	}

	start := time.Date(y, time.Month(m), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, -1)
	const pad = 45 * 24 * time.Hour
	return start.Add(-pad), end.Add(pad), nil
}
