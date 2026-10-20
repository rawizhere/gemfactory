package translate

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"google.golang.org/genai"
)

// Model availability statuses.
const (
	StatusUnknown     = "unknown"
	StatusOK          = "ok"
	StatusDead        = "dead"
	StatusRateLimited = "rate_limited"
	StatusSlow        = "slow"
	StatusError       = "error"
)

// CatalogModel is one Gemini model that supports generateContent.
type CatalogModel struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ContextLength int    `json:"context_length"`
	Status        string `json:"status"`
}

// ModelStatus is the availability result for one model.
type ModelStatus struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

const (
	catalogTTL      = 10 * time.Minute
	pingTimeout     = 20 * time.Second
	keyCheckTimeout = 4 * time.Second
)

type statusEntry struct {
	status    string
	checkedAt time.Time
}

var (
	catalogMu        sync.Mutex
	catalogModels    []CatalogModel
	catalogFetchedAt time.Time
	statusCache      = map[string]statusEntry{}
)

// Models returns the cached (or, with refresh, freshly listed) Gemini models with their last known statuses.
func Models(ctx context.Context, key string, refresh bool) ([]CatalogModel, error) {
	catalogMu.Lock()
	cached, fresh := catalogModels, time.Since(catalogFetchedAt) < catalogTTL
	catalogMu.Unlock()

	models := cached
	if refresh || !fresh {
		var err error
		if models, err = listModels(ctx, key); err != nil {
			return nil, err
		}
		catalogMu.Lock()
		catalogModels, catalogFetchedAt = models, time.Now()
		catalogMu.Unlock()
	}

	out := slices.Clone(models)
	catalogMu.Lock()
	defer catalogMu.Unlock()
	for i := range out {
		out[i].Status = StatusUnknown
		if e, ok := statusCache[out[i].ID]; ok && time.Since(e.checkedAt) < catalogTTL {
			out[i].Status = e.status
		}
	}
	return out, nil
}

func listModels(ctx context.Context, key string) ([]CatalogModel, error) {
	client, err := newClient(ctx, key, 0)
	if err != nil {
		return nil, err
	}
	var out []CatalogModel
	for m, err := range client.Models.All(ctx) {
		if err != nil {
			return nil, fmt.Errorf("gemini list: %w", err)
		}
		if !slices.Contains(m.SupportedActions, "generateContent") {
			continue
		}
		out = append(out, CatalogModel{
			ID:            strings.TrimPrefix(m.Name, "models/"),
			Name:          m.DisplayName,
			ContextLength: int(m.InputTokenLimit),
		})
	}
	return out, nil
}

// CheckModels pings each model (4 at a time) and caches the resulting statuses.
func CheckModels(ctx context.Context, key string, ids []string) []ModelStatus {
	results := make([]ModelStatus, len(ids))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			status := pingModel(ctx, key, id)
			results[i] = ModelStatus{ID: id, Status: status}
			catalogMu.Lock()
			statusCache[id] = statusEntry{status: status, checkedAt: time.Now()}
			catalogMu.Unlock()
		})
	}
	wg.Wait()
	return results
}

func pingModel(ctx context.Context, key, id string) string {
	client, err := newClient(ctx, key, pingTimeout)
	if err != nil {
		return StatusError
	}
	_, err = client.Models.GenerateContent(ctx, id, genai.Text("ping"), &genai.GenerateContentConfig{MaxOutputTokens: 1})
	if err == nil {
		return StatusOK
	}
	var apiErr genai.APIError
	if errors.As(err, &apiErr) {
		if keyInvalid(apiErr) {
			return StatusError
		}
		switch apiErr.Code {
		case http.StatusBadRequest, http.StatusNotFound:
			return StatusDead
		case http.StatusTooManyRequests:
			return StatusRateLimited
		}
	}
	if isTimeout(err) {
		return StatusSlow
	}
	return StatusError
}

// KeyRejected reports whether Gemini refuses key; it is false for an empty key or when the check cannot run.
func KeyRejected(ctx context.Context, key string) bool {
	if key == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, keyCheckTimeout)
	defer cancel()
	client, err := newClient(ctx, key, keyCheckTimeout)
	if err != nil {
		return false
	}
	_, err = client.Models.Get(ctx, DefaultModel, nil)
	var apiErr genai.APIError
	return errors.As(err, &apiErr) && keyInvalid(apiErr)
}

func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	var urlErr *url.Error
	return errors.As(err, &urlErr) && urlErr.Timeout()
}
