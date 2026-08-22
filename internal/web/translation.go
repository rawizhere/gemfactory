package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"gemfactory/internal/downloader"
	"go.uber.org/zap"
)

type translationConfigResponse struct {
	Provider            string `json:"provider"`
	HasGeminiKey        bool   `json:"has_gemini_key"`
	HasGroqKey          bool   `json:"has_groq_key"`
	GeminiMasked        string `json:"gemini_masked"`
	GroqMasked          string `json:"groq_masked"`
	GeminiModels        string `json:"gemini_models"`
	GroqModels          string `json:"groq_models"`
	FallbackOrder       string `json:"fallback_order"`
	DefaultGeminiModels string `json:"default_gemini_models"`
	DefaultGroqModels   string `json:"default_groq_models"`
	Prompt              string `json:"prompt"`
	DefaultPrompt       string `json:"default_prompt"`
	Concurrency         int    `json:"concurrency"`
}

func (s *Server) getTranslationConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	provider := strings.ToLower(strings.TrimSpace(os.Getenv("TRANSLATION_PROVIDER")))
	if provider == "" {
		provider = downloader.ProviderGoogle
	}
	geminiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	groqKey := strings.TrimSpace(os.Getenv("GROQ_API_KEY"))
	geminiModels := strings.TrimSpace(os.Getenv("GEMINI_MODELS"))
	groqModels := strings.TrimSpace(os.Getenv("GROQ_MODELS"))
	fallbackOrder := strings.TrimSpace(os.Getenv("TRANSLATION_FALLBACK_ORDER"))
	prompt := strings.TrimSpace(os.Getenv("TRANSLATION_PROMPT"))
	concurrency := 4
	if s.downloads != nil {
		concurrency = s.downloads.Concurrency()
	}

	if s.configs != nil {
		if c, err := s.configs.Get(ctx, "TRANSLATION_PROVIDER"); err == nil && c != nil && c.Value != "" {
			provider = strings.ToLower(strings.TrimSpace(c.Value))
		}
		if c, err := s.configs.Get(ctx, "GEMINI_API_KEY"); err == nil && c != nil && c.Value != "" {
			geminiKey = strings.TrimSpace(c.Value)
		}
		if c, err := s.configs.Get(ctx, "GROQ_API_KEY"); err == nil && c != nil && c.Value != "" {
			groqKey = strings.TrimSpace(c.Value)
		}
		if c, err := s.configs.Get(ctx, "GEMINI_MODELS"); err == nil && c != nil && strings.TrimSpace(c.Value) != "" {
			geminiModels = strings.TrimSpace(c.Value)
		}
		if c, err := s.configs.Get(ctx, "GROQ_MODELS"); err == nil && c != nil && strings.TrimSpace(c.Value) != "" {
			groqModels = strings.TrimSpace(c.Value)
		}
		if c, err := s.configs.Get(ctx, "TRANSLATION_FALLBACK_ORDER"); err == nil && c != nil && strings.TrimSpace(c.Value) != "" {
			fallbackOrder = strings.TrimSpace(c.Value)
		}
		if c, err := s.configs.Get(ctx, "TRANSLATION_PROMPT"); err == nil && c != nil && strings.TrimSpace(c.Value) != "" {
			prompt = strings.TrimSpace(c.Value)
		}
		if c, err := s.configs.Get(ctx, "DOWNLOAD_CONCURRENCY"); err == nil && c != nil {
			if n, perr := strconv.Atoi(c.Value); perr == nil && n > 0 {
				concurrency = n
			}
		}
	}

	if geminiModels == "" {
		geminiModels = strings.Join(downloader.DefaultGeminiModels, ", ")
	}
	if groqModels == "" {
		groqModels = strings.Join(downloader.DefaultGroqModels, ", ")
	}
	if prompt == "" {
		prompt = downloader.DefaultTranslationPrompt
	}

	resp := translationConfigResponse{
		Provider:            provider,
		HasGeminiKey:        geminiKey != "",
		HasGroqKey:          groqKey != "",
		GeminiMasked:        maskKey(geminiKey),
		GroqMasked:          maskKey(groqKey),
		GeminiModels:        geminiModels,
		GroqModels:          groqModels,
		FallbackOrder:       fallbackOrder,
		DefaultGeminiModels: strings.Join(downloader.DefaultGeminiModels, ", "),
		DefaultGroqModels:   strings.Join(downloader.DefaultGroqModels, ", "),
		Prompt:              prompt,
		DefaultPrompt:       downloader.DefaultTranslationPrompt,
		Concurrency:         concurrency,
	}
	writeJSON(w, resp)
}

func (s *Server) updateTranslationConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider      *string `json:"provider"`
		GeminiKey     *string `json:"gemini_api_key"`
		GroqKey       *string `json:"groq_api_key"`
		GeminiModels  *string `json:"gemini_models"`
		GroqModels    *string `json:"groq_models"`
		FallbackOrder *string `json:"fallback_order"`
		Prompt        *string `json:"prompt"`
		Concurrency   *int    `json:"concurrency"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	if req.Provider != nil {
		p := strings.ToLower(strings.TrimSpace(*req.Provider))
		if p != downloader.ProviderGoogle && p != downloader.ProviderGemini && p != downloader.ProviderGroq {
			http.Error(w, "invalid provider (must be google, gemini, or groq)", http.StatusBadRequest)
			return
		}
		if err := s.configs.Set(ctx, "TRANSLATION_PROVIDER", p); err != nil {
			s.fail(w, err)
			return
		}
	}

	if req.GeminiKey != nil {
		k := strings.TrimSpace(*req.GeminiKey)
		if !strings.Contains(k, "•••") {
			if err := s.configs.Set(ctx, "GEMINI_API_KEY", k); err != nil {
				s.fail(w, err)
				return
			}
		}
	}

	if req.GroqKey != nil {
		k := strings.TrimSpace(*req.GroqKey)
		if !strings.Contains(k, "•••") {
			if err := s.configs.Set(ctx, "GROQ_API_KEY", k); err != nil {
				s.fail(w, err)
				return
			}
		}
	}

	if req.GeminiModels != nil {
		gm := strings.TrimSpace(*req.GeminiModels)
		if err := s.configs.Set(ctx, "GEMINI_MODELS", gm); err != nil {
			s.fail(w, err)
			return
		}
	}

	if req.GroqModels != nil {
		qm := strings.TrimSpace(*req.GroqModels)
		if err := s.configs.Set(ctx, "GROQ_MODELS", qm); err != nil {
			s.fail(w, err)
			return
		}
	}

	if req.FallbackOrder != nil {
		fo := strings.TrimSpace(*req.FallbackOrder)
		if err := s.configs.Set(ctx, "TRANSLATION_FALLBACK_ORDER", fo); err != nil {
			s.fail(w, err)
			return
		}
	}

	if req.Prompt != nil {
		promptVal := strings.TrimSpace(*req.Prompt)
		if err := s.configs.Set(ctx, "TRANSLATION_PROMPT", promptVal); err != nil {
			s.fail(w, err)
			return
		}
	}

	if req.Concurrency != nil && *req.Concurrency > 0 {
		cVal := *req.Concurrency
		if cVal > 20 {
			cVal = 20
		}
		if s.downloads != nil {
			s.downloads.SetConcurrency(cVal)
		}
		if err := s.configs.Set(ctx, "DOWNLOAD_CONCURRENCY", strconv.Itoa(cVal)); err != nil {
			s.fail(w, err)
			return
		}
	}

	s.logger.Info("Settings updated")
	writeJSON(w, map[string]any{"status": "ok"})
}

func (s *Server) testTranslation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider     string `json:"provider"`
		Text         string `json:"text"`
		TargetLang   string `json:"target_lang"`
		GeminiKey    string `json:"gemini_api_key"`
		GroqKey      string `json:"groq_api_key"`
		GeminiModels string `json:"gemini_models"`
		GroqModels   string `json:"groq_models"`
		Prompt       string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	targetLang := strings.TrimSpace(req.TargetLang)
	if targetLang == "" {
		targetLang = "ru"
	}
	sampleText := strings.TrimSpace(req.Text)
	if sampleText == "" {
		sampleText = "대박! 오늘 무대 진짜 레전드였어!"
	}

	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	if provider == "" {
		provider = downloader.ProviderGoogle
	}

	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		if c, err := s.configs.Get(ctx, "TRANSLATION_PROMPT"); err == nil && c != nil && strings.TrimSpace(c.Value) != "" {
			prompt = strings.TrimSpace(c.Value)
		}
		if prompt == "" {
			prompt = downloader.DefaultTranslationPrompt
		}
	}

	geminiKey := strings.TrimSpace(req.GeminiKey)
	if geminiKey == "" || strings.Contains(geminiKey, "•••") {
		if c, err := s.configs.Get(ctx, "GEMINI_API_KEY"); err == nil && c != nil {
			geminiKey = c.Value
		}
		if geminiKey == "" {
			geminiKey = os.Getenv("GEMINI_API_KEY")
		}
	}

	groqKey := strings.TrimSpace(req.GroqKey)
	if groqKey == "" || strings.Contains(groqKey, "•••") {
		if c, err := s.configs.Get(ctx, "GROQ_API_KEY"); err == nil && c != nil {
			groqKey = c.Value
		}
		if groqKey == "" {
			groqKey = os.Getenv("GROQ_API_KEY")
		}
	}

	var results []string
	var err error

	parseModels := func(s string) []string {
		var list []string
		for _, part := range strings.Split(s, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				list = append(list, part)
			}
		}
		return list
	}

	switch provider {
	case downloader.ProviderGemini:
		if geminiKey == "" {
			writeJSON(w, map[string]any{"success": false, "error": "Gemini API key is required"})
			return
		}
		var gModels []string
		if req.GeminiModels != "" {
			gModels = parseModels(req.GeminiModels)
		} else if c, cErr := s.configs.Get(ctx, "GEMINI_MODELS"); cErr == nil && c != nil && c.Value != "" {
			gModels = parseModels(c.Value)
		}
		results, err = downloader.TranslateWithGemini(ctx, []string{sampleText}, targetLang, geminiKey, prompt, gModels)
	case downloader.ProviderGroq:
		if groqKey == "" {
			writeJSON(w, map[string]any{"success": false, "error": "Groq API key is required"})
			return
		}
		var qModels []string
		if req.GroqModels != "" {
			qModels = parseModels(req.GroqModels)
		} else if c, cErr := s.configs.Get(ctx, "GROQ_MODELS"); cErr == nil && c != nil && c.Value != "" {
			qModels = parseModels(c.Value)
		}
		results, err = downloader.TranslateWithGroq(ctx, []string{sampleText}, targetLang, groqKey, prompt, qModels)
	case downloader.ProviderGoogle:
		results, err = downloader.TranslateWithGoogle(ctx, []string{sampleText}, targetLang)
	default:
		writeJSON(w, map[string]any{"success": false, "error": fmt.Sprintf("unknown provider %q", provider)})
		return
	}

	if err != nil {
		s.logger.Warn("Translation test failed", zap.String("provider", provider), zap.Error(err))
		writeJSON(w, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	resultText := ""
	if len(results) > 0 {
		resultText = results[0]
	}

	writeJSON(w, map[string]any{
		"success":  true,
		"provider": provider,
		"input":    sampleText,
		"result":   resultText,
	})
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
