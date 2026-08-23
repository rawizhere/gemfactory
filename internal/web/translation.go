package web

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"gemfactory/internal/downloader"
	"gemfactory/internal/settings"
)

type translationConfigResponse struct {
	Chain                 string `json:"chain"`
	FallbackOrder         string `json:"fallback_order"`
	GeminiMasked          string `json:"gemini_masked"`
	GroqMasked            string `json:"groq_masked"`
	OpencodeMasked        string `json:"opencode_masked"`
	NvidiaMasked          string `json:"nvidia_masked"`
	GeminiModels          string `json:"gemini_models"`
	GroqModels            string `json:"groq_models"`
	OpencodeModels        string `json:"opencode_models"`
	NvidiaModels          string `json:"nvidia_models"`
	DefaultGeminiModels   string `json:"default_gemini_models"`
	DefaultGroqModels     string `json:"default_groq_models"`
	DefaultOpencodeModels string `json:"default_opencode_models"`
	DefaultNvidiaModels   string `json:"default_nvidia_models"`
	Prompt                string `json:"prompt"`
	DefaultPrompt         string `json:"default_prompt"`
	SourcePrefRU          string `json:"source_pref_ru"`
	Concurrency           int    `json:"concurrency"`
	ClipCRF               string `json:"clip_crf"`
	SubsCRF               string `json:"subs_crf"`
	ClipPreset            string `json:"clip_preset"`
	ClipAudioBitrate      string `json:"clip_audio_bitrate"`
	ClipDeleteStatus      bool   `json:"clip_delete_status"`
}

func (s *Server) translationConfig() downloader.TranslationConfig {
	if s.downloads != nil {
		return s.downloads.ResolveTranslationConfig(context.Background())
	}
	return downloader.DefaultTranslationConfig()
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

	resp := translationConfigResponse{
		Chain:                 chainLabel(tc),
		FallbackOrder:         strings.Join(tc.FallbackOrder, ", "),
		GeminiMasked:          maskKey(tc.GeminiKey),
		GroqMasked:            maskKey(tc.GroqKey),
		OpencodeMasked:        maskKey(tc.OpencodeKey),
		NvidiaMasked:          maskKey(tc.NvidiaKey),
		GeminiModels:          strings.Join(tc.GeminiModels, ", "),
		GroqModels:            strings.Join(tc.GroqModels, ", "),
		OpencodeModels:        strings.Join(tc.OpencodeModels, ", "),
		NvidiaModels:          strings.Join(tc.NvidiaModels, ", "),
		DefaultGeminiModels:   strings.Join(downloader.DefaultGeminiModels, ", "),
		DefaultGroqModels:     strings.Join(downloader.DefaultGroqModels, ", "),
		DefaultOpencodeModels: strings.Join(downloader.DefaultOpencodeModels, ", "),
		DefaultNvidiaModels:   strings.Join(downloader.DefaultNvidiaModels, ", "),
		Prompt:                tc.Prompt,
		DefaultPrompt:         downloader.DefaultTranslationPrompt,
		SourcePrefRU:          strings.Join(tc.SourcePrefRU, ", "),
		Concurrency:           concurrency,
		ClipCRF:               clipCRF,
		SubsCRF:               subsCRF,
		ClipPreset:            clipPreset,
		ClipAudioBitrate:      clipAudioBitrate,
		ClipDeleteStatus:      clipDeleteStatus,
	}
	writeJSON(w, resp)
}

// chainLabel renders the effective provider fallback chain for display.
func chainLabel(cfg downloader.TranslationConfig) string {
	names := map[string]string{
		downloader.ProviderGoogle:   "Google Translate",
		downloader.ProviderGemini:   "Gemini",
		downloader.ProviderGroq:     "Groq",
		downloader.ProviderOpencode: "OpenCode",
		downloader.ProviderNvidia:   "NVIDIA",
	}
	var parts []string
	for _, p := range downloader.BuildFallbackChain(cfg) {
		parts = append(parts, names[p])
	}
	return strings.Join(parts, " ➔ ")
}

var (
	validPresets = map[string]bool{
		"ultrafast": true, "superfast": true, "veryfast": true, "faster": true,
		"fast": true, "medium": true, "slow": true,
	}
	bitrateRe = regexp.MustCompile(`^\d+k$`)
)

func validCRF(s string) bool {
	n, err := strconv.Atoi(s)
	return err == nil && n >= 15 && n <= 35
}

func (s *Server) updateTranslationConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GeminiKey        *string `json:"gemini_api_key"`
		GroqKey          *string `json:"groq_api_key"`
		OpencodeKey      *string `json:"opencode_api_key"`
		NvidiaKey        *string `json:"nvidia_api_key"`
		GeminiModels     *string `json:"gemini_models"`
		GroqModels       *string `json:"groq_models"`
		OpencodeModels   *string `json:"opencode_models"`
		NvidiaModels     *string `json:"nvidia_models"`
		FallbackOrder    *string `json:"fallback_order"`
		SourcePrefRU     *string `json:"source_pref_ru"`
		Prompt           *string `json:"prompt"`
		Concurrency      *int    `json:"concurrency"`
		ClipCRF          *string `json:"clip_crf"`
		SubsCRF          *string `json:"subs_crf"`
		ClipPreset       *string `json:"clip_preset"`
		ClipAudioBitrate *string `json:"clip_audio_bitrate"`
		ClipDeleteStatus *bool   `json:"clip_delete_status"`
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

	s.logger.Info("Settings updated")
	writeJSON(w, map[string]any{"status": "ok"})
}

// testTranslation probes the saved fallback chain in parallel and streams results as NDJSON lines.
func (s *Server) testTranslation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text           string `json:"text"`
		TargetLang     string `json:"target_lang"`
		VideoTitle     string `json:"video_title"`
		GeminiKey      string `json:"gemini_api_key"`
		GroqKey        string `json:"groq_api_key"`
		GeminiModels   string `json:"gemini_models"`
		GroqModels     string `json:"groq_models"`
		OpencodeKey    string `json:"opencode_api_key"`
		OpencodeModels string `json:"opencode_models"`
		NvidiaKey      string `json:"nvidia_api_key"`
		NvidiaModels   string `json:"nvidia_models"`
		Prompt         string `json:"prompt"`
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
		sampleText = "대박! 오늘 무대 진짜 레전드였어!"
	}
	videoTitle := strings.TrimSpace(req.VideoTitle)

	tc := s.translationConfig()
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
	if req.GeminiModels != "" {
		tc.GeminiModels = downloader.ParseCSV(req.GeminiModels)
	}
	if req.GroqModels != "" {
		tc.GroqModels = downloader.ParseCSV(req.GroqModels)
	}
	if req.OpencodeModels != "" {
		tc.OpencodeModels = downloader.ParseCSV(req.OpencodeModels)
	}
	if req.NvidiaModels != "" {
		tc.NvidiaModels = downloader.ParseCSV(req.NvidiaModels)
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		prompt = tc.Prompt
	}
	instr := downloader.BuildSystemInstruction(prompt, targetLang)

	chain := downloader.BuildFallbackChain(tc)
	extract := func(texts []string, _, err error, entry map[string]any) map[string]any {
		switch {
		case err != nil:
			entry["error"] = err.Error()
		case !downloader.TranslationLooksTarget([]string{sampleText}, texts, targetLang):
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
		case downloader.ProviderGoogle:
			testModel(p, "web", func() ([]string, string, error) {
				res, err := downloader.TranslateWithGoogle(ctx, []string{sampleText}, targetLang)
				return res, "web", err
			})
		case downloader.ProviderGemini:
			if tc.GeminiKey == "" {
				noKey(p)
				continue
			}
			for _, m := range tc.GeminiModels {
				testModel(p, m, func() ([]string, string, error) {
					return downloader.TranslateWithGemini(ctx, []string{sampleText}, targetLang, "", tc.GeminiKey, instr, videoTitle, []string{m})
				})
			}
		case downloader.ProviderGroq:
			if tc.GroqKey == "" {
				noKey(p)
				continue
			}
			for _, m := range tc.GroqModels {
				testModel(p, m, func() ([]string, string, error) {
					return downloader.TranslateWithGroq(ctx, []string{sampleText}, targetLang, "", tc.GroqKey, instr, videoTitle, []string{m})
				})
			}
		case downloader.ProviderOpencode:
			if tc.OpencodeKey == "" {
				noKey(p)
				continue
			}
			for _, m := range tc.OpencodeModels {
				testModel(p, m, func() ([]string, string, error) {
					return downloader.TranslateWithOpencode(ctx, []string{sampleText}, targetLang, "", tc.OpencodeKey, instr, videoTitle, []string{m})
				})
			}
		case downloader.ProviderNvidia:
			if tc.NvidiaKey == "" {
				noKey(p)
				continue
			}
			for _, m := range tc.NvidiaModels {
				testModel(p, m, func() ([]string, string, error) {
					return downloader.TranslateWithNvidia(ctx, []string{sampleText}, targetLang, "", tc.NvidiaKey, instr, videoTitle, []string{m})
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
	w.Write(append(b, '\n'))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// effectiveKey treats a masked input value as "keep the stored key".
func (s *Server) effectiveKey(r *http.Request, provided string, cfgKey string) string {
	if provided != "" && !isMaskedKey(provided) {
		return provided
	}
	ctx := r.Context()
	if s.configs != nil {
		if c, err := s.configs.Get(ctx, cfgKey); err == nil && c != nil && strings.TrimSpace(c.Value) != "" {
			return strings.TrimSpace(c.Value)
		}
	}
	return strings.TrimSpace(os.Getenv(cfgKey))
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
