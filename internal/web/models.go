package web

import (
	"encoding/json"
	"net/http"

	"gemfactory/internal/translate"
)

// keyForProvider returns the configured API key for a catalog provider.
func keyForProvider(tc translate.Config, provider string) string {
	switch provider {
	case translate.ProviderGroq:
		return tc.GroqKey
	case translate.ProviderOpencode:
		return tc.OpencodeKey
	case translate.ProviderNvidia:
		return tc.NvidiaKey
	case translate.ProviderGemini:
		return tc.GeminiKey
	case translate.ProviderOpenRouter:
		return tc.OpenRouterKey
	}
	return ""
}

// getModels returns a provider catalog with cached availability statuses.
func (s *Server) getModels(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		http.Error(w, "provider query parameter is required", http.StatusBadRequest)
		return
	}
	refresh := r.URL.Query().Get("refresh") == "true"
	apiKey := keyForProvider(s.translationConfig(), provider)

	models, err := translate.GetProviderCatalog(r.Context(), provider, apiKey, refresh)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, map[string]any{"provider": provider, "models": models})
}

// checkModels pings the requested models in parallel and returns their statuses.
func (s *Server) checkModels(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string   `json:"provider"`
		Models   []string `json:"models"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Provider == "" {
		http.Error(w, "provider is required", http.StatusBadRequest)
		return
	}
	if len(req.Models) == 0 {
		http.Error(w, "models list is required", http.StatusBadRequest)
		return
	}
	apiKey := keyForProvider(s.translationConfig(), req.Provider)

	statuses, err := translate.CheckModelsAvailability(r.Context(), req.Provider, apiKey, req.Models)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, statuses)
}
