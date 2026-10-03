package scraper

import (
	"strings"

	"go.uber.org/zap"
)

// Fetcher parses release calendars from kpopofficial.com.
type Fetcher struct {
	logger     *zap.Logger
	httpClient *HTTPClient
}

func NewFetcher(userAgents []string, logger *zap.Logger) *Fetcher {
	return &Fetcher{
		logger:     logger,
		httpClient: NewHTTPClient(userAgents, logger),
	}
}

// ParseUserAgents splits a multiline setting into a non-empty agent list.
func ParseUserAgents(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			out = append(out, line)
		}
	}
	return out
}

// SetUserAgents updates the scraper user agent pool at runtime.
func (f *Fetcher) SetUserAgents(userAgents []string) {
	f.httpClient.SetUserAgents(userAgents)
}
