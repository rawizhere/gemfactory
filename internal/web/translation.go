package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gemfactory/internal/settings"
	"gemfactory/internal/translate"
)

func (s *Server) translationConfig(ctx context.Context) translate.Config {
	return translate.ResolveConfig(ctx, settings.New(s.Configs))
}

// getTranslationConfig reports which configured provider keys are rejected (401/403).
func (s *Server) getTranslationConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"broken_keys": translate.CheckBrokenKeys(r.Context(), s.translationConfig(r.Context()))})
}

// restrictTestToModel narrows the translation test to a single provider/model.
// The UI sends "provider/model" (e.g. "opencode/nemotron-3.5-lightning-free"); when set,
// only that provider is probed and its model list is reduced to the chosen model.
func restrictTestToModel(tc *translate.Config, chain []string, model string) []string {
	provider, m, ok := strings.Cut(model, "/")
	if !ok || !translate.IsProvider(provider) {
		return chain
	}
	if tc.Models == nil {
		tc.Models = map[string][]string{}
	}
	tc.Models[provider] = []string{m}
	return []string{provider}
}

// testTranslation probes the fallback chain in parallel and streams results as NDJSON lines.
// The body is a flat string object: text, target_lang, video_title, fallback_order, prompt,
// model, plus per-provider <provider>_api_key and <provider>_models overrides.
func (s *Server) testTranslation(w http.ResponseWriter, r *http.Request) {
	var req map[string]string
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()

	targetLang := strings.TrimSpace(req["target_lang"])
	if targetLang == "" {
		targetLang = "ru"
	}
	sampleText := strings.TrimSpace(req["text"])
	if sampleText == "" {
		sampleText = "리브 미모 난리도 아니야"
	}
	videoTitle := strings.TrimSpace(req["video_title"])

	tc := s.translationConfig(ctx)
	if v := req["fallback_order"]; v != "" {
		tc.FallbackOrder = translate.ParseCSV(v)
	}
	for _, p := range translate.AllProviders() {
		if k := req[p+"_api_key"]; k != "" && !isMaskedKey(k) {
			tc.Keys[p] = k
		}
		if m := req[p+"_models"]; m != "" {
			tc.Models[p] = translate.ParseCSV(m)
		}
	}
	if prompt := strings.TrimSpace(req["prompt"]); prompt != "" {
		tc.Prompt = prompt
	}

	chain := restrictTestToModel(&tc, translate.BuildFallbackChain(tc), req["model"])

	type testItem struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}

	var items []testItem
	for _, p := range chain {
		models := tc.Models[p]
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

			key := tc.Keys[item.Provider]
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
			itemCfg := tc
			itemCfg.Models = map[string][]string{item.Provider: {item.Model}}
			texts, _, err := translate.Translate(ctx, item.Provider, itemCfg, []string{sampleText}, targetLang, "", videoTitle, nil)

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
