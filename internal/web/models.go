package web

import (
	"encoding/json"
	"net/http"

	"gemfactory/internal/translate"
)

// getModels returns a provider catalog with cached availability statuses.
func (s *Server) getModels(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		http.Error(w, "provider query parameter is required", http.StatusBadRequest)
		return
	}
	refresh := r.URL.Query().Get("refresh") == "true"
	apiKey := s.translationConfig(r.Context()).Keys[provider]

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
	apiKey := s.translationConfig(r.Context()).Keys[req.Provider]

	statuses, err := translate.CheckModelsAvailability(r.Context(), req.Provider, apiKey, req.Models)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, statuses)
}
