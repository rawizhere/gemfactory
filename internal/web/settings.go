package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"gemfactory/internal/translate"
)

// settingSpec describes one web-manageable config key.
type settingSpec struct {
	key      string
	def      string                // displayed when unset in db and env
	masked   bool                  // API-key style value
	validate func(string) error    // nil = free-form
	apply    func(*Server, string) // runtime side effect, optional
}

var bitrateRe = regexp.MustCompile(`^\d+k$`)

func intRange(min, max int) func(string) error {
	return func(v string) error {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || n < min || n > max {
			return fmt.Errorf("must be an integer between %d and %d", min, max)
		}
		return nil
	}
}

func oneOf(allowed ...string) func(string) error {
	set := make(map[string]bool, len(allowed))
	for _, a := range allowed {
		set[a] = true
	}
	return func(v string) error {
		if !set[strings.TrimSpace(v)] {
			return fmt.Errorf("must be one of: %s", strings.Join(allowed, ", "))
		}
		return nil
	}
}

func boolSpec() func(string) error {
	return oneOf("true", "false")
}

func csvOf(allowed ...string) func(string) error {
	set := make(map[string]bool, len(allowed))
	for _, a := range allowed {
		set[a] = true
	}
	return func(v string) error {
		for _, part := range strings.Split(v, ",") {
			p := strings.ToLower(strings.TrimSpace(part))
			if p != "" && !set[p] {
				return fmt.Errorf("unknown value %q", part)
			}
		}
		return nil
	}
}

func normalizeBool(v string) string {
	if strings.EqualFold(strings.TrimSpace(v), "true") {
		return "true"
	}
	return "false"
}

func settingRegistry() []settingSpec {
	return []settingSpec{
		{key: "DOWNLOAD_CONCURRENCY", def: "4", validate: intRange(1, 20),
			apply: func(s *Server, v string) {
				if n, err := strconv.Atoi(v); err == nil && s.downloads != nil {
					s.downloads.SetConcurrency(n)
				}
			}},
		{key: "DOWNLOAD_RETENTION_HOURS", def: "24", validate: intRange(1, 8760)},
		{key: "TG_FILE_LIMIT_MB", def: "49", validate: intRange(1, 2048)},
		{key: "YTDLP_PROXY", def: ""},
		{key: "CLIP_CRF", def: "20", validate: intRange(15, 35)},
		{key: "SUBS_CRF", def: "20", validate: intRange(15, 35)},
		{key: "CLIP_PRESET", def: "fast", validate: oneOf("ultrafast", "superfast", "veryfast", "faster", "fast", "medium", "slow")},
		{key: "CLIP_AUDIO_BITRATE", def: "192k", validate: func(v string) error {
			if !bitrateRe.MatchString(strings.TrimSpace(v)) {
				return fmt.Errorf("must look like 192k")
			}
			return nil
		}},
		{key: "CLIP_DELETE_STATUS", def: "false", validate: boolSpec()},
		{key: "TRANSLATION_TIMEOUT", def: "180", validate: intRange(10, 600)},
		{key: "SUBS_GOOGLE_ONLY", def: "false", validate: boolSpec()},
		{key: "TRANSLATION_FALLBACK_ORDER", def: "openrouter,opencode,nvidia,groq",
			validate: csvOf(translate.ProviderGoogle, translate.ProviderGemini, translate.ProviderNvidia, translate.ProviderGroq, translate.ProviderOpencode, translate.ProviderOpenRouter)},
		{key: "SUBS_SOURCE_PREF_RU", def: "en,ko"},
		{key: "TRANSLATION_PROMPT", def: translate.DefaultTranslationPrompt},
		{key: "GEMINI_API_KEY", masked: true},
		{key: "GROQ_API_KEY", masked: true},
		{key: "OPENCODE_API_KEY", masked: true},
		{key: "NVIDIA_API_KEY", masked: true},
		{key: "OPENROUTER_API_KEY", masked: true},
		{key: "GEMINI_MODELS", def: strings.Join(translate.DefaultGeminiModels, ",")},
		{key: "GROQ_MODELS", def: strings.Join(translate.DefaultGroqModels, ",")},
		{key: "OPENCODE_MODELS", def: strings.Join(translate.DefaultOpencodeModels, ",")},
		{key: "NVIDIA_MODELS", def: strings.Join(translate.DefaultNvidiaModels, ",")},
		{key: "OPENROUTER_MODELS", def: strings.Join(translate.DefaultOpenRouterModels, ",")},
	}
}

// resolveSetting returns the effective value and its source ("db", "env" or "default").
func (s *Server) resolveSetting(ctx context.Context, spec settingSpec) (value, source string) {
	if s.configs != nil {
		if c, err := s.configs.Get(ctx, spec.key); err == nil && c != nil && strings.TrimSpace(c.Value) != "" {
			return strings.TrimSpace(c.Value), "db"
		}
	}
	if env := os.Getenv(spec.key); strings.TrimSpace(env) != "" {
		return strings.TrimSpace(env), "env"
	}
	return spec.def, "default"
}

// GET /api/settings — effective values with source info plus read-only system info.
func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	type entry struct {
		Key     string `json:"key"`
		Value   string `json:"value"`
		Source  string `json:"source"`
		Masked  bool   `json:"masked"`
		Default string `json:"default"`
	}
	out := make([]entry, 0, 24)
	for _, spec := range settingRegistry() {
		value, source := s.resolveSetting(ctx, spec)
		shown := value
		if spec.masked && shown != "" {
			shown = maskKey(shown)
		}
		out = append(out, entry{
			Key:     spec.key,
			Value:   shown,
			Source:  source,
			Masked:  spec.masked,
			Default: spec.def,
		})
	}

	system := []map[string]string{}
	if s.appCfg != nil {
		system = append(system,
			map[string]string{"key": "WEB_PORT", "value": s.appCfg.WebPort},
			map[string]string{"key": "HEALTH_PORT", "value": s.appCfg.HealthPort},
			map[string]string{"key": "WEB_ENABLED", "value": strconv.FormatBool(s.appCfg.WebEnabled)},
			map[string]string{"key": "HEALTH_CHECK_ENABLED", "value": strconv.FormatBool(s.appCfg.HealthCheckEnabled)},
			map[string]string{"key": "LOG_LEVEL", "value": s.appCfg.LogLevel},
			map[string]string{"key": "TIMEZONE", "value": s.appCfg.Timezone},
			map[string]string{"key": "APP_DATA_DIR", "value": s.appCfg.AppDataDir},
			map[string]string{"key": "SCRAPER_REQUEST_DELAY", "value": s.appCfg.ScraperDelay.String()},
			map[string]string{"key": "RELEASE_CHECK_INTERVAL", "value": s.appCfg.ReleaseCheckInterval.String()},
			map[string]string{"key": "FFMPEG_BINARY", "value": os.Getenv("FFMPEG_BINARY")},
		)
	}

	writeJSON(w, map[string]any{"settings": out, "system": system})
}

// POST /api/settings — partial update; validates everything before applying anything.
func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) {
	var req map[string]string
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	specs := settingRegistry()
	byKey := make(map[string]settingSpec, len(specs))
	for _, spec := range specs {
		byKey[spec.key] = spec
	}

	ctx := r.Context()
	type pending struct {
		spec settingSpec
		val  string
	}
	var updates []pending
	for key, raw := range req {
		spec, ok := byKey[key]
		if !ok {
			http.Error(w, "unknown setting "+key, http.StatusBadRequest)
			return
		}
		v := strings.TrimSpace(raw)

		if spec.masked {
			if v == "" || isMaskedKey(v) {
				continue // empty or echoed-back mask = keep current
			}
		}

		if spec.validate != nil {
			if err := spec.validate(v); err != nil {
				http.Error(w, fmt.Sprintf("%s: %v", key, err), http.StatusBadRequest)
				return
			}
		}
		if spec.key == "CLIP_DELETE_STATUS" || spec.key == "SUBS_GOOGLE_ONLY" {
			v = normalizeBool(v)
		}
		updates = append(updates, pending{spec: spec, val: v})
	}

	for _, u := range updates {
		if err := s.configs.Set(ctx, u.spec.key, u.val); err != nil {
			s.fail(w, err)
			return
		}
	}
	for _, u := range updates {
		if u.spec.apply != nil {
			u.spec.apply(s, u.val)
		}
	}

	s.logger.Info("Settings updated", zap.Int("count", len(updates)))
	writeJSON(w, map[string]any{"status": "ok"})
}
