package scraper

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"

	"gemfactory/internal/config"
)

// uaCooldown is how long a user agent is skipped after a 403.
const uaCooldown = time.Hour

type HTTPClient struct {
	client     *http.Client
	logger     *zap.Logger
	nextAt     time.Time // earliest time the next request may start, guarded by mu
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
		Timeout:   60 * time.Second,
	}

	if len(userAgents) == 0 {
		userAgents = config.DefaultScraperUserAgents
	}

	return &HTTPClient{
		client:     client,
		logger:     logger,
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

// requestDelay spaces requests to kpopofficial.com.
const requestDelay = 2 * time.Second

// wait blocks until requestDelay has passed since the previous request.
func (c *HTTPClient) wait(ctx context.Context) error {
	c.mu.Lock()
	now := time.Now()
	start := now
	if c.nextAt.After(now) {
		start = c.nextAt
	}
	c.nextAt = start.Add(requestDelay)
	c.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(start.Sub(now)):
		return nil
	}
}

// doRequest sends the request, failing over to the next user agent on 403.
// A timeout is retried once: cold year windows generate slowly, the retry usually hits the warm cache.
func (c *HTTPClient) doRequest(ctx context.Context, apiURL string) (*http.Response, error) {
	c.mu.Lock()
	attempts := len(c.userAgents)
	c.mu.Unlock()
	if attempts == 0 {
		return nil, fmt.Errorf("user agent pool is empty")
	}

	resp, err := c.doRequestOnce(ctx, apiURL, attempts)
	if err != nil && timeoutErr(err) {
		c.logger.Warn("REST request timed out, retrying once", zap.String("url", apiURL))
		resp, err = c.doRequestOnce(ctx, apiURL, attempts)
	}
	return resp, err
}

// timeoutErr reports client timeout, request timeout and context deadline errors.
func timeoutErr(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout() || errors.Is(err, context.DeadlineExceeded)
}

func (c *HTTPClient) doRequestOnce(ctx context.Context, apiURL string, attempts int) (*http.Response, error) {
	for attempt := 0; attempt < attempts; attempt++ {
		if err := c.wait(ctx); err != nil {
			return nil, err
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
