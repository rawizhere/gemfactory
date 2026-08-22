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

type albumTitle struct {
	Rendered string `json:"rendered"`
}

type albumContent struct {
	Rendered string `json:"rendered"`
}

type AlbumPost struct {
	Link    string       `json:"link"`
	Title   albumTitle   `json:"title"`
	Content albumContent `json:"content"`
}

const albumsPerPage = 100

func (c *HTTPClient) FetchAlbumsWindow(ctx context.Context, after, before time.Time) ([]AlbumPost, error) {
	var all []AlbumPost

	for page := 1; ; page++ {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limit wait failed: %w", err)
		}

		q := url.Values{}
		q.Set("per_page", strconv.Itoa(albumsPerPage))
		q.Set("page", strconv.Itoa(page))
		if !after.IsZero() {
			q.Set("after", after.Format(time.RFC3339))
		}
		if !before.IsZero() {
			q.Set("before", before.Format(time.RFC3339))
		}
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

		if resp.StatusCode == http.StatusBadRequest {
			// WordPress returns 400 rest_post_invalid_page_number when page > total_pages
			_ = resp.Body.Close()
			break
		}

		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("unexpected rest status code: %d", resp.StatusCode)
		}

		totalPages := 0
		if tpHeader := resp.Header.Get("X-WP-TotalPages"); tpHeader != "" {
			if tp, convErr := strconv.Atoi(tpHeader); convErr == nil {
				totalPages = tp
			}
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

		all = append(all, posts...)

		if (totalPages > 0 && page >= totalPages) || len(posts) < albumsPerPage {
			break
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}

	return all, nil
}

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

func yearWindow(year string) (time.Time, time.Time, error) {
	y, err := strconv.Atoi(strings.TrimSpace(year))
	if err != nil || y <= 0 {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid year: %q", year)
	}

	start := time.Date(y, time.January, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(y, time.December, 31, 23, 59, 59, 0, time.UTC)
	return start.AddDate(0, 0, -30), end.AddDate(0, 0, 60), nil
}
