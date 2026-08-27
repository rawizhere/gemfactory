package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"gemfactory/internal/settings"
	"gemfactory/internal/translate"
)

type translationConfigResponse struct {
	Chain                   string          `json:"chain"`
	FallbackOrder           string          `json:"fallback_order"`
	GeminiMasked            string          `json:"gemini_masked"`
	GroqMasked              string          `json:"groq_masked"`
	OpencodeMasked          string          `json:"opencode_masked"`
	NvidiaMasked            string          `json:"nvidia_masked"`
	OpenRouterMasked        string          `json:"openrouter_masked"`
	BrokenKeys              map[string]bool `json:"broken_keys"`
	GeminiModels            string          `json:"gemini_models"`
	GroqModels              string          `json:"groq_models"`
	OpencodeModels          string          `json:"opencode_models"`
	NvidiaModels            string          `json:"nvidia_models"`
	OpenRouterModels        string          `json:"openrouter_models"`
	DefaultGeminiModels     string          `json:"default_gemini_models"`
	DefaultGroqModels       string          `json:"default_groq_models"`
	DefaultOpencodeModels   string          `json:"default_opencode_models"`
	DefaultNvidiaModels     string          `json:"default_nvidia_models"`
	DefaultOpenRouterModels string          `json:"default_openrouter_models"`
	Prompt                  string          `json:"prompt"`
	DefaultPrompt           string          `json:"default_prompt"`
	SourcePrefRU            string          `json:"source_pref_ru"`
	Concurrency             int             `json:"concurrency"`
	ClipCRF                 string          `json:"clip_crf"`
	SubsCRF                 string          `json:"subs_crf"`
	ClipPreset              string          `json:"clip_preset"`
	ClipAudioBitrate        string          `json:"clip_audio_bitrate"`
	ClipDeleteStatus        bool            `json:"clip_delete_status"`
	RetentionHours          int             `json:"retention_hours"`
	TranslationTimeout      int             `json:"translation_timeout"`
}

func (s *Server) translationConfig() translate.Config {
	if s.downloads != nil {
		return s.downloads.ResolveTranslationConfig(context.Background())
	}
	return translate.DefaultConfig()
}

func (s *Server) getTranslationConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tc := s.translationConfig()
	cfg := settings.New(s.configs)

	concurrency := cfg.Int(ctx, "DOWNLOAD_CONCURRENCY", 4)
	clipCRF := cfg.Value(ctx, "CLIP_CRF", "20")
	subsCRF := cfg.Value(ctx, "SUBS_CRF", "20")
	clipPreset := cfg.Value(ctx, "CLIP_PRESET", "fast")
	clipAudioBitrate := cfg.Value(ctx, "CLIP_AUDIO_BITRATE", "192k")
	clipDeleteStatus := cfg.Bool(ctx, "CLIP_DELETE_STATUS", false)
	retentionHours := cfg.Int(ctx, "DOWNLOAD_RETENTION_HOURS", 24)
	translationTimeout := int(tc.Timeout.Seconds())
	resp := translationConfigResponse{
		Chain:                   chainLabel(tc),
		FallbackOrder:           strings.Join(tc.FallbackOrder, ", "),
		GeminiMasked:            maskKey(tc.GeminiKey),
		GroqMasked:              maskKey(tc.GroqKey),
		OpencodeMasked:          maskKey(tc.OpencodeKey),
		NvidiaMasked:            maskKey(tc.NvidiaKey),
		OpenRouterMasked:        maskKey(tc.OpenRouterKey),
		BrokenKeys:              brokenKeys(r, tc),
		GeminiModels:            strings.Join(tc.GeminiModels, ", "),
		GroqModels:              strings.Join(tc.GroqModels, ", "),
		OpencodeModels:          strings.Join(tc.OpencodeModels, ", "),
		NvidiaModels:            strings.Join(tc.NvidiaModels, ", "),
		OpenRouterModels:        strings.Join(tc.OpenRouterModels, ", "),
		DefaultGeminiModels:     strings.Join(translate.DefaultGeminiModels, ", "),
		DefaultGroqModels:       strings.Join(translate.DefaultGroqModels, ", "),
		DefaultOpencodeModels:   strings.Join(translate.DefaultOpencodeModels, ", "),
		DefaultNvidiaModels:     strings.Join(translate.DefaultNvidiaModels, ", "),
		DefaultOpenRouterModels: strings.Join(translate.DefaultOpenRouterModels, ", "),
		Prompt:                  tc.Prompt,
		DefaultPrompt:           translate.DefaultTranslationPrompt,
		SourcePrefRU:            strings.Join(tc.SourcePrefRU, ", "),
		Concurrency:             concurrency,
		ClipCRF:                 clipCRF,
		SubsCRF:                 subsCRF,
		ClipPreset:              clipPreset,
		ClipAudioBitrate:        clipAudioBitrate,
		ClipDeleteStatus:        clipDeleteStatus,
		RetentionHours:          retentionHours,
		TranslationTimeout:      translationTimeout,
	}
	writeJSON(w, resp)
}

// brokenKeys probes configured provider keys for 401/403 when the caller opts in via ?health=1.
func brokenKeys(r *http.Request, tc translate.Config) map[string]bool {
	if r.URL.Query().Get("health") != "1" {
		return nil
	}
	return translate.CheckBrokenKeys(r.Context(), tc)
}

// chainLabel renders the effective provider fallback chain for display.
func chainLabel(cfg translate.Config) string {
	names := map[string]string{
		translate.ProviderGoogle:     "Google Translate",
		translate.ProviderGemini:     "Gemini",
		translate.ProviderGroq:       "Groq",
		translate.ProviderOpencode:   "OpenCode",
		translate.ProviderNvidia:     "NVIDIA",
		translate.ProviderOpenRouter: "OpenRouter",
	}
	var parts []string
	for _, p := range translate.BuildFallbackChain(cfg) {
		parts = append(parts, names[p])
	}
	return strings.Join(parts, " ➔ ")
}

var validPresets = map[string]bool{
	"ultrafast": true, "superfast": true, "veryfast": true, "faster": true,
	"fast": true, "medium": true, "slow": true,
}

func validCRF(s string) bool {
	n, err := strconv.Atoi(s)
	return err == nil && n >= 15 && n <= 35
}

func (s *Server) updateTranslationConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GeminiKey          *string `json:"gemini_api_key"`
		GroqKey            *string `json:"groq_api_key"`
		OpencodeKey        *string `json:"opencode_api_key"`
		NvidiaKey          *string `json:"nvidia_api_key"`
		OpenRouterKey      *string `json:"openrouter_api_key"`
		GeminiModels       *string `json:"gemini_models"`
		GroqModels         *string `json:"groq_models"`
		OpencodeModels     *string `json:"opencode_models"`
		NvidiaModels       *string `json:"nvidia_models"`
		OpenRouterModels   *string `json:"openrouter_models"`
		FallbackOrder      *string `json:"fallback_order"`
		SourcePrefRU       *string `json:"source_pref_ru"`
		Prompt             *string `json:"prompt"`
		Concurrency        *int    `json:"concurrency"`
		ClipCRF            *string `json:"clip_crf"`
		SubsCRF            *string `json:"subs_crf"`
		ClipPreset         *string `json:"clip_preset"`
		ClipAudioBitrate   *string `json:"clip_audio_bitrate"`
		ClipDeleteStatus   *bool   `json:"clip_delete_status"`
		RetentionHours     *int    `json:"retention_hours"`
		TranslationTimeout *int    `json:"translation_timeout"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	set := func(key, value string) bool {
		if err := s.configs.Set(ctx, key, value); err != nil {
			s.fail(w, err)
			return false
		}
		return true
	}
	setKey := func(cfgKey string, provided *string) bool {
		k := strings.TrimSpace(*provided)
		if isMaskedKey(k) {
			return true
		}
		return set(cfgKey, k)
	}

	if req.GeminiKey != nil && !setKey("GEMINI_API_KEY", req.GeminiKey) {
		return
	}
	if req.GroqKey != nil && !setKey("GROQ_API_KEY", req.GroqKey) {
		return
	}
	if req.OpencodeKey != nil && !setKey("OPENCODE_API_KEY", req.OpencodeKey) {
		return
	}
	if req.NvidiaKey != nil && !setKey("NVIDIA_API_KEY", req.NvidiaKey) {
		return
	}
	if req.OpenRouterKey != nil && !setKey("OPENROUTER_API_KEY", req.OpenRouterKey) {
		return
	}

	if req.ClipCRF != nil {
		if v := strings.TrimSpace(*req.ClipCRF); v != "" {
			if !validCRF(v) {
				http.Error(w, "invalid CRF (must be 15-35)", http.StatusBadRequest)
				return
			}
			if !set("CLIP_CRF", v) {
				return
			}
		}
	}
	if req.SubsCRF != nil {
		if v := strings.TrimSpace(*req.SubsCRF); v != "" {
			if !validCRF(v) {
				http.Error(w, "invalid CRF (must be 15-35)", http.StatusBadRequest)
				return
			}
			if !set("SUBS_CRF", v) {
				return
			}
		}
	}
	if req.ClipPreset != nil {
		if v := strings.TrimSpace(*req.ClipPreset); v != "" {
			if !validPresets[v] {
				http.Error(w, "invalid x264 preset", http.StatusBadRequest)
				return
			}
			if !set("CLIP_PRESET", v) {
				return
			}
		}
	}
	if req.ClipAudioBitrate != nil {
		if v := strings.TrimSpace(*req.ClipAudioBitrate); v != "" {
			if !bitrateRe.MatchString(v) {
				http.Error(w, "invalid audio bitrate (must look like 192k)", http.StatusBadRequest)
				return
			}
			if !set("CLIP_AUDIO_BITRATE", v) {
				return
			}
		}
	}

	if req.GeminiModels != nil && !set("GEMINI_MODELS", strings.TrimSpace(*req.GeminiModels)) {
		return
	}
	if req.GroqModels != nil && !set("GROQ_MODELS", strings.TrimSpace(*req.GroqModels)) {
		return
	}
	if req.OpencodeModels != nil && !set("OPENCODE_MODELS", strings.TrimSpace(*req.OpencodeModels)) {
		return
	}
	if req.NvidiaModels != nil && !set("NVIDIA_MODELS", strings.TrimSpace(*req.NvidiaModels)) {
		return
	}
	if req.OpenRouterModels != nil && !set("OPENROUTER_MODELS", strings.TrimSpace(*req.OpenRouterModels)) {
		return
	}
	if req.FallbackOrder != nil && !set("TRANSLATION_FALLBACK_ORDER", strings.TrimSpace(*req.FallbackOrder)) {
		return
	}
	if req.SourcePrefRU != nil && !set("SUBS_SOURCE_PREF_RU", strings.TrimSpace(*req.SourcePrefRU)) {
		return
	}
	if req.Prompt != nil && !set("TRANSLATION_PROMPT", strings.TrimSpace(*req.Prompt)) {
		return
	}

	if req.Concurrency != nil && *req.Concurrency > 0 {
		cVal := min(*req.Concurrency, 20)
		if s.downloads != nil {
			s.downloads.SetConcurrency(cVal)
		}
		if !set("DOWNLOAD_CONCURRENCY", strconv.Itoa(cVal)) {
			return
		}
	}

	if req.ClipDeleteStatus != nil {
		val := "false"
		if *req.ClipDeleteStatus {
			val = "true"
		}
		if !set("CLIP_DELETE_STATUS", val) {
			return
		}
	}

	if req.RetentionHours != nil {
		h := *req.RetentionHours
		if h < 1 || h > 8760 {
			http.Error(w, "retention must be between 1 and 8760 hours", http.StatusBadRequest)
			return
		}
		if !set("DOWNLOAD_RETENTION_HOURS", strconv.Itoa(h)) {
			return
		}
	}

	if req.TranslationTimeout != nil {
		sec := *req.TranslationTimeout
		if sec < 10 || sec > 600 {
			http.Error(w, "translation timeout must be between 10 and 600 seconds", http.StatusBadRequest)
			return
		}
		if !set("TRANSLATION_TIMEOUT", strconv.Itoa(sec)) {
			return
		}
	}

	s.logger.Info("Settings updated")
	writeJSON(w, map[string]any{"status": "ok"})
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
	case translate.ProviderGoogle:
		// Google web translate has no model id; the provider restriction is enough.
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

// testTranslation probes the saved fallback chain in parallel and streams results as NDJSON lines.
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
	instr := translate.BuildSystemInstruction(prompt, targetLang)

	chain := restrictTestToModel(&tc, translate.BuildFallbackChain(tc), req.Model)

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

	w.Header().Set("Content-Type", "application/x-ndjson")
	writeNDJSON(w, map[string]any{"type": "start", "chain": chain})

	results := make(chan map[string]any, 64)
	var wg sync.WaitGroup

	testModel := func(provider, model string, fn func() ([]string, string, error)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			texts, _, err := fn()
			results <- extract(texts, nil, err, map[string]any{
				"type": "result", "provider": provider, "model": model, "ok": false,
			})
		}()
	}
	noKey := func(provider string) {
		results <- map[string]any{"type": "result", "provider": provider, "model": "", "ok": false, "error": "no API key configured"}
	}

	for _, p := range chain {
		switch p {
		case translate.ProviderGoogle:
			testModel(p, "web", func() ([]string, string, error) {
				res, err := translate.TranslateWithGoogle(ctx, []string{sampleText}, targetLang)
				return res, "web", err
			})
		case translate.ProviderGemini:
			if tc.GeminiKey == "" {
				noKey(p)
				continue
			}
			for _, m := range tc.GeminiModels {
				testModel(p, m, func() ([]string, string, error) {
					return translate.TranslateWithGemini(ctx, []string{sampleText}, targetLang, "", tc.GeminiKey, instr, videoTitle, tc.Timeout, nil, []string{m})
				})
			}
		case translate.ProviderGroq:
			if tc.GroqKey == "" {
				noKey(p)
				continue
			}
			for _, m := range tc.GroqModels {
				testModel(p, m, func() ([]string, string, error) {
					return translate.TranslateWithGroq(ctx, []string{sampleText}, targetLang, "", tc.GroqKey, instr, videoTitle, tc.Timeout, nil, []string{m})
				})
			}
		case translate.ProviderOpencode:
			if tc.OpencodeKey == "" {
				noKey(p)
				continue
			}
			for _, m := range tc.OpencodeModels {
				testModel(p, m, func() ([]string, string, error) {
					return translate.TranslateWithOpencode(ctx, []string{sampleText}, targetLang, "", tc.OpencodeKey, instr, videoTitle, tc.Timeout, nil, []string{m})
				})
			}
		case translate.ProviderNvidia:
			if tc.NvidiaKey == "" {
				noKey(p)
				continue
			}
			for _, m := range tc.NvidiaModels {
				testModel(p, m, func() ([]string, string, error) {
					return translate.TranslateWithNvidia(ctx, []string{sampleText}, targetLang, "", tc.NvidiaKey, instr, videoTitle, tc.Timeout, nil, []string{m})
				})
			}
		case translate.ProviderOpenRouter:
			if tc.OpenRouterKey == "" {
				noKey(p)
				continue
			}
			for _, m := range tc.OpenRouterModels {
				testModel(p, m, func() ([]string, string, error) {
					return translate.TranslateWithOpenRouter(ctx, []string{sampleText}, targetLang, "", tc.OpenRouterKey, instr, videoTitle, tc.Timeout, nil, []string{m})
				})
			}
		}
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	okCount := 0
	for e := range results {
		if e["ok"] == true {
			okCount++
		}
		writeNDJSON(w, e)
	}

	writeNDJSON(w, map[string]any{"type": "done", "success": okCount > 0})
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
