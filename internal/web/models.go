package web

import (
	"encoding/json"
	"net/http"

	"gemfactory/internal/translate"
)

// getModels returns the Gemini model catalog with cached availability statuses.
func (s *Server) getModels(w http.ResponseWriter, r *http.Request) {
	refresh := r.URL.Query().Get("refresh") == "true"
	models, err := translate.Models(r.Context(), s.translationConfig(r.Context()).Key, refresh)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, map[string]any{"models": models})
}

// checkModels pings the requested models in parallel and returns their statuses.
func (s *Server) checkModels(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Models []string `json:"models"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.Models) == 0 {
		http.Error(w, "models list is required", http.StatusBadRequest)
		return
	}
	writeJSON(w, translate.CheckModels(r.Context(), s.translationConfig(r.Context()).Key, req.Models))
}
