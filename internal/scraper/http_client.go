package scraper

import (
	"net/http"
	"time"

	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

type HTTPClient struct {
	client    *http.Client
	logger    *zap.Logger
	userAgent string
	limiter   *rate.Limiter
}

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
