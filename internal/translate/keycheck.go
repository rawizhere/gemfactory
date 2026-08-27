package translate

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/sashabaranov/go-openai"
	"google.golang.org/genai"
)

// KeyCheckTimeout bounds a single provider key probe.
const KeyCheckTimeout = 4 * time.Second

// keyFor returns the configured API key for a provider, or "" when none is set.
func (c Config) keyFor(provider string) string {
	switch provider {
	case ProviderGemini:
		return c.GeminiKey
	case ProviderGroq:
		return c.GroqKey
	case ProviderOpencode:
		return c.OpencodeKey
	case ProviderNvidia:
		return c.NvidiaKey
	case ProviderOpenRouter:
		return c.OpenRouterKey
	}
	return ""
}

// ProbeKey does one lightweight request against the provider using its configured
// key and reports whether the key was rejected with 401/403. It returns false when
// no key is configured or the probe could not run (network error, timeout, etc.).
func ProbeKey(ctx context.Context, provider string, cfg Config) bool {
	if cfg.keyFor(provider) == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, KeyCheckTimeout)
	defer cancel()

	switch provider {
	case ProviderGemini:
		return probeGeminiKey(ctx, cfg.GeminiKey)
	case ProviderGroq:
		return probeOpenAIKey(ctx, groqBaseURL, cfg.GroqKey)
	case ProviderOpencode:
		return probeOpenAIKey(ctx, opencodeBaseURL, cfg.OpencodeKey)
	case ProviderNvidia:
		return probeOpenAIKey(ctx, nvidiaBaseURL, cfg.NvidiaKey)
	case ProviderOpenRouter:
		return probeOpenAIKey(ctx, openRouterBaseURL, cfg.OpenRouterKey)
	}
	return false
}

// CheckBrokenKeys probes every provider that has a configured key and returns the
// set of providers whose key was rejected with 401/403. Providers without a key
// are omitted. The whole sweep is bounded by the caller's context.
func CheckBrokenKeys(ctx context.Context, cfg Config) map[string]bool {
	providers := []string{ProviderGemini, ProviderGroq, ProviderOpencode, ProviderNvidia, ProviderOpenRouter}
	out := make(map[string]bool, len(providers))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, p := range providers {
		if cfg.keyFor(p) == "" {
			continue
		}
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			broken := ProbeKey(ctx, p, cfg)
			mu.Lock()
			out[p] = broken
			mu.Unlock()
		}(p)
	}
	wg.Wait()
	return out
}

func probeOpenAIKey(ctx context.Context, baseURL, apiKey string) bool {
	c := openai.DefaultConfig(apiKey)
	c.BaseURL = baseURL
	c.HTTPClient = &http.Client{Timeout: KeyCheckTimeout}
	client := openai.NewClientWithConfig(c)
	_, err := client.ListModels(ctx)
	if err == nil {
		return false
	}
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) && isAuthStatus(apiErr.HTTPStatusCode) {
		return true
	}
	return false
}

func probeGeminiKey(ctx context.Context, apiKey string) bool {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:     apiKey,
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: &http.Client{Timeout: KeyCheckTimeout},
	})
	if err != nil {
		return false
	}
	_, err = client.Models.Get(ctx, DefaultGeminiModels[0], nil)
	if err == nil {
		return false
	}
	var apiErr genai.APIError
	if errors.As(err, &apiErr) && isAuthStatus(apiErr.Code) {
		return true
	}
	return false
}

func isAuthStatus(code int) bool {
	return code == http.StatusUnauthorized || code == http.StatusForbidden
}
