package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gemfactory/internal/translate"
)

func (s *Server) translationConfig() translate.Config {
	if s.Downloads != nil {
		return s.Downloads.ResolveTranslationConfig(context.Background())
	}
	return translate.DefaultConfig()
}

// getTranslationConfig reports which configured provider keys are rejected (401/403).
func (s *Server) getTranslationConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"broken_keys": translate.CheckBrokenKeys(r.Context(), s.translationConfig())})
}

// restrictTestToModel narrows the translation test to a single provider/model.
// The UI sends "provider/model" (e.g. "opencode/nemotron-3.5-lightning-free"); when set,
// only that provider is probed and its model list is reduced to the chosen model.
func restrictTestToModel(tc *translate.Config, chain []string, model string) []string {
	if model == "" {
		return chain
	}
	parts := strings.SplitN(model, "/", 2)
	if len(parts) != 2 {
		return chain
	}
	provider, m := parts[0], parts[1]
	switch provider {
	case translate.ProviderGemini:
		tc.GeminiModels = []string{m}
	case translate.ProviderGroq:
		tc.GroqModels = []string{m}
	case translate.ProviderOpencode:
		tc.OpencodeModels = []string{m}
	case translate.ProviderNvidia:
		tc.NvidiaModels = []string{m}
	case translate.ProviderOpenRouter:
		tc.OpenRouterModels = []string{m}
	default:
		return chain
	}
	restricted := []string{}
	for _, p := range chain {
		if p == provider {
			restricted = append(restricted, p)
		}
	}
	if len(restricted) == 0 {
		restricted = []string{provider}
	}
	return restricted
}

// testTranslation probes the fallback chain in parallel and streams results as NDJSON lines.
func (s *Server) testTranslation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text             string `json:"text"`
		TargetLang       string `json:"target_lang"`
		VideoTitle       string `json:"video_title"`
		FallbackOrder    string `json:"fallback_order"`
		GeminiKey        string `json:"gemini_api_key"`
		GroqKey          string `json:"groq_api_key"`
		GeminiModels     string `json:"gemini_models"`
		GroqModels       string `json:"groq_models"`
		OpencodeKey      string `json:"opencode_api_key"`
		OpencodeModels   string `json:"opencode_models"`
		NvidiaKey        string `json:"nvidia_api_key"`
		NvidiaModels     string `json:"nvidia_models"`
		OpenRouterKey    string `json:"openrouter_api_key"`
		OpenRouterModels string `json:"openrouter_models"`
		Prompt           string `json:"prompt"`
		Model            string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()

	targetLang := strings.TrimSpace(req.TargetLang)
	if targetLang == "" {
		targetLang = "ru"
	}
	sampleText := strings.TrimSpace(req.Text)
	if sampleText == "" {
		sampleText = "리브 미모 난리도 아니야"
	}
	videoTitle := strings.TrimSpace(req.VideoTitle)

	tc := s.translationConfig()
	if req.FallbackOrder != "" {
		tc.FallbackOrder = translate.ParseCSV(req.FallbackOrder)
	}
	if req.GeminiKey != "" && !isMaskedKey(req.GeminiKey) {
		tc.GeminiKey = req.GeminiKey
	}
	if req.GroqKey != "" && !isMaskedKey(req.GroqKey) {
		tc.GroqKey = req.GroqKey
	}
	if req.OpencodeKey != "" && !isMaskedKey(req.OpencodeKey) {
		tc.OpencodeKey = req.OpencodeKey
	}
	if req.NvidiaKey != "" && !isMaskedKey(req.NvidiaKey) {
		tc.NvidiaKey = req.NvidiaKey
	}
	if req.OpenRouterKey != "" && !isMaskedKey(req.OpenRouterKey) {
		tc.OpenRouterKey = req.OpenRouterKey
	}
	if req.GeminiModels != "" {
		tc.GeminiModels = translate.ParseCSV(req.GeminiModels)
	}
	if req.GroqModels != "" {
		tc.GroqModels = translate.ParseCSV(req.GroqModels)
	}
	if req.OpencodeModels != "" {
		tc.OpencodeModels = translate.ParseCSV(req.OpencodeModels)
	}
	if req.NvidiaModels != "" {
		tc.NvidiaModels = translate.ParseCSV(req.NvidiaModels)
	}
	if req.OpenRouterModels != "" {
		tc.OpenRouterModels = translate.ParseCSV(req.OpenRouterModels)
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		prompt = tc.Prompt
	}
	instr := prompt

	chain := restrictTestToModel(&tc, translate.BuildFallbackChain(tc), req.Model)

	type testItem struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}

	var items []testItem
	for _, p := range chain {
		var models []string
		switch p {
		case translate.ProviderGemini:
			models = tc.GeminiModels
		case translate.ProviderGroq:
			models = tc.GroqModels
		case translate.ProviderOpencode:
			models = tc.OpencodeModels
		case translate.ProviderNvidia:
			models = tc.NvidiaModels
		case translate.ProviderOpenRouter:
			models = tc.OpenRouterModels
		}
		if len(models) == 0 {
			items = append(items, testItem{Provider: p, Model: ""})
		} else {
			for _, m := range models {
				items = append(items, testItem{Provider: p, Model: m})
			}
		}
	}
	if items == nil {
		items = []testItem{}
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	var writeMu sync.Mutex
	sendNDJSON := func(v any) {
		writeMu.Lock()
		defer writeMu.Unlock()
		writeNDJSON(w, v)
	}

	sendNDJSON(map[string]any{"type": "init", "chain": chain, "items": items})

	extract := func(texts []string, _, err error, entry map[string]any) map[string]any {
		switch {
		case err != nil:
			entry["error"] = err.Error()
		case !translate.TranslationLooksTarget([]string{sampleText}, texts, targetLang):
			entry["error"] = "output rejected by language validation"
			if len(texts) > 0 {
				entry["result"] = texts[0]
			}
		default:
			entry["ok"] = true
			if len(texts) > 0 {
				entry["result"] = texts[0]
			}
		}
		return entry
	}

	var okCount int64
	var wg sync.WaitGroup

	for _, it := range items {
		item := it
		wg.Add(1)
		go func() {
			defer wg.Done()

			if ctx.Err() != nil {
				sendNDJSON(map[string]any{
					"type":       "result",
					"provider":   item.Provider,
					"model":      item.Model,
					"ok":         false,
					"latency_ms": 0,
					"error":      ctx.Err().Error(),
				})
				return
			}

			var key string
			switch item.Provider {
			case translate.ProviderGemini:
				key = tc.GeminiKey
			case translate.ProviderGroq:
				key = tc.GroqKey
			case translate.ProviderOpencode:
				key = tc.OpencodeKey
			case translate.ProviderNvidia:
				key = tc.NvidiaKey
			case translate.ProviderOpenRouter:
				key = tc.OpenRouterKey
			}

			if key == "" {
				sendNDJSON(map[string]any{
					"type":       "result",
					"provider":   item.Provider,
					"model":      item.Model,
					"ok":         false,
					"latency_ms": 0,
					"error":      "no API key configured",
				})
				return
			}

			start := time.Now()
			var texts []string
			var err error

			switch item.Provider {
			case translate.ProviderGemini:
				texts, _, err = translate.TranslateWithGemini(ctx, []string{sampleText}, targetLang, "", key, instr, videoTitle, tc.Timeout, nil, []string{item.Model})
			case translate.ProviderGroq:
				texts, _, err = translate.TranslateWithGroq(ctx, []string{sampleText}, targetLang, "", key, instr, videoTitle, tc.Timeout, nil, []string{item.Model})
			case translate.ProviderOpencode:
				texts, _, err = translate.TranslateWithOpencode(ctx, []string{sampleText}, targetLang, "", key, instr, videoTitle, tc.Timeout, nil, []string{item.Model})
			case translate.ProviderNvidia:
				texts, _, err = translate.TranslateWithNvidia(ctx, []string{sampleText}, targetLang, "", key, instr, videoTitle, tc.Timeout, nil, []string{item.Model})
			case translate.ProviderOpenRouter:
				texts, _, err = translate.TranslateWithOpenRouter(ctx, []string{sampleText}, targetLang, "", key, instr, videoTitle, tc.Timeout, nil, []string{item.Model})
			}

			latencyMs := int(time.Since(start).Milliseconds())
			entry := extract(texts, nil, err, map[string]any{
				"type":       "result",
				"provider":   item.Provider,
				"model":      item.Model,
				"ok":         false,
				"latency_ms": latencyMs,
			})
			if entry["ok"] == true {
				atomic.AddInt64(&okCount, 1)
			}
			sendNDJSON(entry)
		}()
	}

	wg.Wait()
	sendNDJSON(map[string]any{"type": "done", "success": atomic.LoadInt64(&okCount) > 0})
}

func writeNDJSON(w http.ResponseWriter, v any) {
	b, _ := json.Marshal(v)
	_, _ = w.Write(append(b, '\n'))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
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
