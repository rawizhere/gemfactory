package scraper

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/PuerkitoBio/goquery"
	"go.uber.org/zap"
)

// HTTPClient provides a specialized web client for fetching and parsing HTML documents.
type HTTPClient struct {
	client    *http.Client
	logger    *zap.Logger
	userAgent string
}

// NewHTTPClient initializes a new HTTPClient with custom transport and user-agent settings.
func NewHTTPClient(config HTTPClientConfig, userAgent string, logger *zap.Logger) *HTTPClient {
	transport := &http.Transport{
		MaxIdleConns:          config.MaxIdleConns,
		MaxIdleConnsPerHost:   config.MaxIdleConnsPerHost,
		IdleConnTimeout:       config.IdleConnTimeout,
		TLSHandshakeTimeout:   config.TLSHandshakeTimeout,
		ResponseHeaderTimeout: config.ResponseHeaderTimeout,
		DisableKeepAlives:     config.DisableKeepAlives,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	return &HTTPClient{
		client:    client,
		logger:    logger,
		userAgent: userAgent,
	}
}

// GetHTML fetches HTML page and returns goquery document.
func (c *HTTPClient) GetHTML(ctx context.Context, url string) (*goquery.Document, error) {
	if len(url) > 7 && url[:7] == "file://" {
		// Handle local file access for testing or scraping stored content.
		path := url[7:]

		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("failed to open local file: %w", err)
		}
		defer func() {
			if closeErr := f.Close(); closeErr != nil {
				c.logger.Error("Failed to close file", zap.Error(closeErr))
			}
		}()

		doc, err := goquery.NewDocumentFromReader(f)
		if err != nil {
			return nil, fmt.Errorf("failed to parse local HTML: %w", err)
		}
		return doc, nil
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set User-Agent.
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
