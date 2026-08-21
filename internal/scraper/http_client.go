package scraper

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/PuerkitoBio/goquery"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// HTTPClient provides a specialized web client for fetching and parsing HTML documents.
type HTTPClient struct {
	client    *http.Client
	logger    *zap.Logger
	userAgent string
	limiter   *rate.Limiter
}

// NewHTTPClient initializes a new HTTPClient with connection pooling.
func NewHTTPClient(userAgent string, logger *zap.Logger) *HTTPClient {
	transport := &http.Transport{
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	limiter := rate.NewLimiter(rate.Every(2*time.Second), 1)

	return &HTTPClient{
		client:    client,
		logger:    logger,
		userAgent: userAgent,
		limiter:   limiter,
	}
}

// GetHTML fetches an HTML page and returns a goquery document.
func (c *HTTPClient) GetHTML(ctx context.Context, url string) (*goquery.Document, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			c.logger.Error("Failed to close response body", zap.Error(closeErr))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	return doc, nil
}

// CheckStatus checks the HTTP status code of a URL.
func (c *HTTPClient) CheckStatus(ctx context.Context, url string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create check request: %w", err)
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("status check failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode, nil
}
