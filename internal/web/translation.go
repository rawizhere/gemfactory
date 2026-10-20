package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"gemfactory/internal/settings"
	"gemfactory/internal/translate"
)

func (s *Server) translationConfig(ctx context.Context) translate.Config {
	return translate.ResolveConfig(ctx, settings.New(s.Configs))
}

// getTranslationConfig reports whether Gemini refuses the stored key.
func (s *Server) getTranslationConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"key_rejected": translate.KeyRejected(r.Context(), s.translationConfig(r.Context()).Key)})
}

// testTranslation translates one sample line with the stored config, overridden by unsaved panel edits.
func (s *Server) testTranslation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text       string `json:"text"`
		TargetLang string `json:"target_lang"`
		VideoTitle string `json:"video_title"`
		Key        string `json:"api_key"`
		Model      string `json:"model"`
		Prompt     string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()

	tc := s.translationConfig(ctx)
	if k := strings.TrimSpace(req.Key); k != "" && !isMaskedKey(k) {
		tc.Key = k
	}
	if m := strings.TrimSpace(req.Model); m != "" {
		tc.Model = m
	}
	if p := strings.TrimSpace(req.Prompt); p != "" {
		tc.Prompt = p
	}
	targetLang := strings.TrimSpace(req.TargetLang)
	if targetLang == "" {
		targetLang = "ru"
	}
	sample := strings.TrimSpace(req.Text)
	if sample == "" {
		sample = "리브 미모 난리도 아니야"
	}

	start := time.Now()
	texts, err := translate.Translate(ctx, tc, []string{sample}, targetLang, "", strings.TrimSpace(req.VideoTitle))
	out := map[string]any{"model": tc.Model, "latency_ms": time.Since(start).Milliseconds(), "ok": err == nil}
	if err != nil {
		out["error"] = err.Error()
	} else {
		out["result"] = texts[0]
	}
	writeJSON(w, out)
}

// isMaskedKey reports whether the value looks like a masked key echoed back by the UI.
func isMaskedKey(key string) bool {
	return strings.Contains(key, "•")
}

func maskKey(key string) string {
	if len(key) <= 8 {
		if len(key) == 0 {
			return ""
		}
		return "••••••••"
	}
	return key[:6] + "••••••••" + key[len(key)-4:]
}
