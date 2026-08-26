package translate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"google.golang.org/genai"
)

// Model availability / catalog statuses.
const (
	StatusUnknown     = "unknown"
	StatusOK          = "ok"
	StatusDead        = "dead"
	StatusRateLimited = "rate_limited"
	StatusQuota       = "quota"
	StatusNotChat     = "not_chat"
	StatusSlow        = "slow"
	StatusError       = "error"
)

// providerBaseURL maps OpenAI-compatible providers to their API base.
var providerBaseURL = map[string]string{
	ProviderOpenRouter: "https://openrouter.ai/api/v1",
	ProviderOpencode:   opencodeBaseURL,
	ProviderNvidia:     nvidiaBaseURL,
	ProviderGroq:       groqBaseURL,
}

// CatalogModel is one entry in a provider's model catalog.
type CatalogModel struct {
	Provider      string `json:"provider"`
	ID            string `json:"id"`
	Free          bool   `json:"free"`
	ContextLength int    `json:"context_length"`
	Reasoning     bool   `json:"reasoning"`
	Status        string `json:"status"`
}

// ModelStatus is the availability result for one model.
type ModelStatus struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
	Status   string `json:"status"`
}

const catalogTTL = 10 * time.Minute
const pingTimeout = 20 * time.Second

type catalogCacheEntry struct {
	models    []CatalogModel
	fetchedAt time.Time
}

type statusCacheEntry struct {
	status    string
	checkedAt time.Time
}

var (
	catalogMu    sync.Mutex
	catalogCache = map[string]catalogCacheEntry{}
	statusCache  = map[string]map[string]statusCacheEntry{}
)

func isSupportedProvider(p string) bool {
	switch p {
	case ProviderOpenRouter, ProviderOpencode, ProviderNvidia, ProviderGroq, ProviderGemini:
		return true
	}
	return false
}

func httpGet(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "gemfactory")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("catalog request to %s returned %d", u, resp.StatusCode)
	}
	return body, nil
}

func httpGetWithKey(ctx context.Context, u, apiKey string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "gemfactory")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("catalog request to %s returned %d", u, resp.StatusCode)
	}
	return body, nil
}

// openRouterModel is the raw OpenRouter model shape.
type openRouterModel struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ContextLength int    `json:"context_length"`
	Reasoning     *struct {
		DefaultEnabled bool `json:"default_enabled"`
	} `json:"reasoning"`
	Pricing struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
	} `json:"pricing"`
}

// OpenRouterIsFree reports whether an OpenRouter model is free.
func OpenRouterIsFree(m openRouterModel) bool {
	return m.Pricing.Prompt == "0" && m.Pricing.Completion == "0"
}

// OpencodeIsFree reports whether an OpenCode model id denotes a free model.
func OpencodeIsFree(id string) bool {
	return strings.Contains(id, "-free")
}

// ParseOpenRouterModels parses the OpenRouter /models response.
func ParseOpenRouterModels(body []byte) ([]CatalogModel, error) {
	var resp struct {
		Data []openRouterModel `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse openrouter catalog: %w", err)
	}
	out := make([]CatalogModel, 0, len(resp.Data))
	for _, m := range resp.Data {
		reasoning := m.Reasoning != nil && m.Reasoning.DefaultEnabled
		out = append(out, CatalogModel{
			Provider:      ProviderOpenRouter,
			ID:            m.ID,
			Free:          OpenRouterIsFree(m),
			ContextLength: m.ContextLength,
			Reasoning:     reasoning,
		})
	}
	return out, nil
}

// openAIModelID is the id field of an OpenAI-shaped model entry.
type openAIModelID struct {
	ID string `json:"id"`
}

// ParseOpenAIModelIDs parses an OpenAI-compatible /models response into ids.
func ParseOpenAIModelIDs(body []byte) ([]string, error) {
	var resp struct {
		Data []openAIModelID `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse openai catalog: %w", err)
	}
	ids := make([]string, 0, len(resp.Data))
	for _, m := range resp.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	return ids, nil
}

func buildCatalog(provider string, ids []string, freeFn func(string) bool) []CatalogModel {
	out := make([]CatalogModel, 0, len(ids))
	for _, id := range ids {
		free := false
		if freeFn != nil {
			free = freeFn(id)
		}
		out = append(out, CatalogModel{Provider: provider, ID: id, Free: free})
	}
	return out
}

func fetchGeminiModels(ctx context.Context, apiKey string) ([]CatalogModel, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("gemini client: %w", err)
	}
	out := []CatalogModel{}
	page, err := client.Models.List(ctx, &genai.ListModelsConfig{})
	if err != nil {
		return nil, fmt.Errorf("gemini list: %w", err)
	}
	for {
		for _, m := range page.Items {
			out = append(out, CatalogModel{
				Provider:      ProviderGemini,
				ID:            strings.TrimPrefix(m.Name, "models/"),
				ContextLength: int(m.InputTokenLimit),
				Reasoning:     false,
			})
		}
		if page.NextPageToken == "" {
			break
		}
		page, err = client.Models.List(ctx, &genai.ListModelsConfig{PageToken: page.NextPageToken})
		if err != nil {
			return nil, fmt.Errorf("gemini list: %w", err)
		}
	}
	return out, nil
}

func fetchCatalog(ctx context.Context, provider, apiKey string) ([]CatalogModel, error) {
	switch provider {
	case ProviderOpenRouter:
		body, err := httpGet(ctx, providerBaseURL[provider]+"/models")
		if err != nil {
			return nil, err
		}
		return ParseOpenRouterModels(body)
	case ProviderOpencode:
		body, err := httpGet(ctx, providerBaseURL[provider]+"/models")
		if err != nil {
			return nil, err
		}
		ids, err := ParseOpenAIModelIDs(body)
		if err != nil {
			return nil, err
		}
		return buildCatalog(ProviderOpencode, ids, OpencodeIsFree), nil
	case ProviderNvidia:
		body, err := httpGet(ctx, providerBaseURL[provider]+"/models")
		if err != nil {
			return nil, err
		}
		ids, err := ParseOpenAIModelIDs(body)
		if err != nil {
			return nil, err
		}
		return buildCatalog(ProviderNvidia, ids, nil), nil
	case ProviderGroq:
		if apiKey == "" {
			return nil, fmt.Errorf("groq api key required")
		}
		body, err := httpGetWithKey(ctx, providerBaseURL[provider]+"/models", apiKey)
		if err != nil {
			return nil, err
		}
		ids, err := ParseOpenAIModelIDs(body)
		if err != nil {
			return nil, err
		}
		return buildCatalog(ProviderGroq, ids, nil), nil
	case ProviderGemini:
		if apiKey == "" {
			return nil, fmt.Errorf("gemini api key required")
		}
		return fetchGeminiModels(ctx, apiKey)
	}
	return nil, fmt.Errorf("unsupported provider: %s", provider)
}

func cachedCatalog(provider string) ([]CatalogModel, bool) {
	catalogMu.Lock()
	defer catalogMu.Unlock()
	entry, ok := catalogCache[provider]
	if !ok || time.Since(entry.fetchedAt) > catalogTTL {
		return nil, false
	}
	return entry.models, true
}

func storeCatalog(provider string, models []CatalogModel) {
	catalogMu.Lock()
	defer catalogMu.Unlock()
	catalogCache[provider] = catalogCacheEntry{models: models, fetchedAt: time.Now()}
}

func cachedStatus(provider, id string) (string, bool) {
	catalogMu.Lock()
	defer catalogMu.Unlock()
	pm, ok := statusCache[provider]
	if !ok {
		return "", false
	}
	entry, ok := pm[id]
	if !ok || time.Since(entry.checkedAt) > catalogTTL {
		return "", false
	}
	return entry.status, true
}

func storeStatus(provider, id, status string) {
	catalogMu.Lock()
	defer catalogMu.Unlock()
	if statusCache[provider] == nil {
		statusCache[provider] = map[string]statusCacheEntry{}
	}
	statusCache[provider][id] = statusCacheEntry{status: status, checkedAt: time.Now()}
}

func mergeStatuses(provider string, models []CatalogModel) []CatalogModel {
	out := make([]CatalogModel, len(models))
	for i, m := range models {
		m.Status = StatusUnknown
		if st, ok := cachedStatus(provider, m.ID); ok {
			m.Status = st
		}
		out[i] = m
	}
	return out
}

// GetProviderCatalog returns the cached (or freshly fetched) model catalog for a provider.
func GetProviderCatalog(ctx context.Context, provider, apiKey string, refresh bool) ([]CatalogModel, error) {
	if !isSupportedProvider(provider) {
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
	if !refresh {
		if cached, ok := cachedCatalog(provider); ok {
			return mergeStatuses(provider, cached), nil
		}
	}
	models, err := fetchCatalog(ctx, provider, apiKey)
	if err != nil {
		return nil, err
	}
	storeCatalog(provider, models)
	return mergeStatuses(provider, models), nil
}

// CheckModelsAvailability pings each model (concurrency 4) and returns per-model status.
func CheckModelsAvailability(ctx context.Context, provider, apiKey string, modelIDs []string) ([]ModelStatus, error) {
	if !isSupportedProvider(provider) {
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
	results := make([]ModelStatus, len(modelIDs))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, id := range modelIDs {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			status := pingModel(ctx, provider, apiKey, id)
			results[i] = ModelStatus{Provider: provider, ID: id, Status: status}
			storeStatus(provider, id, status)
		}(i, id)
	}
	wg.Wait()
	return results, nil
}

func pingModel(ctx context.Context, provider, apiKey, id string) string {
	if provider == ProviderGemini {
		return pingGemini(ctx, apiKey, id)
	}
	base, ok := providerBaseURL[provider]
	if !ok {
		return StatusError
	}
	return pingOpenAI(ctx, base, apiKey, id)
}

func pingOpenAI(ctx context.Context, baseURL, apiKey, modelID string) string {
	payload := map[string]any{
		"model":      modelID,
		"max_tokens": 1,
		"messages":   []map[string]string{{"role": "user", "content": "ping"}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return StatusError
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return StatusError
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	client := &http.Client{Timeout: pingTimeout}
	resp, err := client.Do(req)
	if err != nil {
		if isTimeout(err) {
			return StatusSlow
		}
		return StatusError
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)
	return classifyOpenAIStatus(resp.StatusCode, string(respBody))
}

func classifyOpenAIStatus(code int, body string) string {
	switch code {
	case http.StatusOK:
		return StatusOK
	case http.StatusBadRequest, http.StatusNotFound:
		return StatusDead
	case http.StatusTooManyRequests:
		if strings.Contains(strings.ToLower(body), "credits") {
			return StatusQuota
		}
		return StatusRateLimited
	case http.StatusForbidden, http.StatusBadGateway:
		return StatusNotChat
	default:
		return StatusError
	}
}

func pingGemini(ctx context.Context, apiKey, modelID string) string {
	if apiKey == "" {
		return StatusError
	}
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return StatusError
	}
	cfg := &genai.GenerateContentConfig{MaxOutputTokens: 1}
	_, err = client.Models.GenerateContent(ctx, modelID, genai.Text("ping"), cfg)
	if err == nil {
		return StatusOK
	}
	code, msg := geminiAPIErrorDetails(err)
	switch code {
	case http.StatusOK:
		return StatusOK
	case http.StatusBadRequest, http.StatusNotFound:
		return StatusDead
	case http.StatusTooManyRequests:
		if strings.Contains(msg, "credits") {
			return StatusQuota
		}
		return StatusRateLimited
	case http.StatusForbidden:
		return StatusNotChat
	default:
		if isTimeout(err) {
			return StatusSlow
		}
		return StatusError
	}
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
	if errors.As(err, &urlErr) && urlErr.Timeout() {
		return true
	}
	return false
}
