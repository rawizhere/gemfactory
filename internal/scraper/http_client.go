package scraper

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/time/rate"

	"gemfactory/internal/config"
)

// uaCooldown is how long a user agent is skipped after a 403.
const uaCooldown = time.Hour

type HTTPClient struct {
	client     *http.Client
	logger     *zap.Logger
	limiter    *rate.Limiter
	mu         sync.Mutex
	userAgents []string
	badUntil   map[string]time.Time
}

func NewHTTPClient(userAgents []string, logger *zap.Logger) *HTTPClient {
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

	if len(userAgents) == 0 {
		userAgents = config.DefaultScraperUserAgents
	}

	return &HTTPClient{
		client:     client,
		logger:     logger,
		limiter:    limiter,
		userAgents: userAgents,
		badUntil:   make(map[string]time.Time),
	}
}

// SetUserAgents replaces the user agent pool and resets failover state.
func (c *HTTPClient) SetUserAgents(userAgents []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(userAgents) == 0 {
		return
	}
	c.userAgents = userAgents
	c.badUntil = make(map[string]time.Time)
}

// pickUA returns the first agent not cooling down, or the one closest to recovery.
func (c *HTTPClient) pickUA() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for _, ua := range c.userAgents {
		if c.badUntil[ua].Before(now) {
			return ua
		}
	}
	best := c.userAgents[0]
	for _, ua := range c.userAgents[1:] {
		if c.badUntil[ua].Before(c.badUntil[best]) {
			best = ua
		}
	}
	return best
}

func (c *HTTPClient) markBad(ua string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.badUntil[ua] = time.Now().Add(uaCooldown)
}

// doRequest sends the request, failing over to the next user agent on 403.
func (c *HTTPClient) doRequest(ctx context.Context, apiURL string) (*http.Response, error) {
	c.mu.Lock()
	attempts := len(c.userAgents)
	c.mu.Unlock()
	if attempts == 0 {
		return nil, fmt.Errorf("user agent pool is empty")
	}

	for attempt := 0; attempt < attempts; attempt++ {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limit wait failed: %w", err)
		}
		ua := c.pickUA()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create rest request: %w", err)
		}
		req.Header.Set("User-Agent", ua)
		req.Header.Set("Accept", "application/json")
		resp, err := c.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("rest request failed: %w", err)
		}
		if resp.StatusCode == http.StatusForbidden && attempt < attempts-1 {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512))
			_ = resp.Body.Close()
			c.markBad(ua)
			c.logger.Warn("Scraper user agent blocked, failing over", zap.String("user_agent", ua))
			continue
		}
		return resp, nil
	}
	return nil, fmt.Errorf("user agent pool exhausted")
}
