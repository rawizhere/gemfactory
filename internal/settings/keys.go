package settings

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"gemfactory/internal/config"
)

// Key names a config value with its default and, for web-editable keys, its validation.
type Key struct {
	Name     string
	Def      string
	Masked   bool               // API-key style value, shown masked
	Validate func(string) error // nil = free-form
}

// Check trims and validates v; an empty value clears the stored one and is always accepted.
func (k Key) Check(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", nil
	}
	if k.Validate != nil {
		if err := k.Validate(v); err != nil {
			return "", err
		}
	}
	return v, nil
}

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
	return func(v string) error {
		if !slices.Contains(allowed, strings.TrimSpace(v)) {
			return fmt.Errorf("must be one of: %s", strings.Join(allowed, ", "))
		}
		return nil
	}
}

func intKey(name string, def, min, max int) Key {
	return Key{Name: name, Def: strconv.Itoa(def), Validate: intRange(min, max)}
}

func boolKey(name string, def bool) Key {
	return Key{Name: name, Def: strconv.FormatBool(def), Validate: oneOf("true", "false")}
}

var bitrateRe = regexp.MustCompile(`^\d+k$`)

// Web-editable keys, in the order the settings panel lists them.
var (
	DownloadConcurrency      = intKey("DOWNLOAD_CONCURRENCY", 4, 1, 20)
	DownloadRetentionHours   = intKey("DOWNLOAD_RETENTION_HOURS", 24, 1, 8760)
	TGFileLimitMB            = intKey("TG_FILE_LIMIT_MB", 49, 1, 2048)
	ClipMaxDurationSeconds   = intKey("CLIP_MAX_DURATION_SECONDS", 300, 5, 3600)
	ClipMaxDurationHQSeconds = intKey("CLIP_MAX_DURATION_HQ_SECONDS", 30, 5, 600)
	YTDLPProxy               = Key{Name: "YTDLP_PROXY"}
	ClipCRF                  = intKey("CLIP_CRF", 20, 15, 35)
	SubsCRF                  = intKey("SUBS_CRF", 20, 15, 35)
	ClipPreset               = Key{Name: "CLIP_PRESET", Def: "fast", Validate: oneOf("ultrafast", "superfast", "veryfast", "faster", "fast", "medium", "slow")}
	ClipAudioBitrate         = Key{Name: "CLIP_AUDIO_BITRATE", Def: "192k", Validate: validateBitrate}
	ClipDeleteStatus         = boolKey("CLIP_DELETE_STATUS", false)
	TranslationTimeout       = intKey("TRANSLATION_TIMEOUT", 180, 10, 600)
	SubsYouTubeOnly          = boolKey("SUBS_YOUTUBE_ONLY", false)
	SubsSourcePrefRU         = Key{Name: "SUBS_SOURCE_PREF_RU", Def: "en,ko"}
	TranslationPrompt        = Key{Name: "TRANSLATION_PROMPT", Def: defaultTranslationPrompt}
	GeminiAPIKey             = Key{Name: "GEMINI_API_KEY", Masked: true}
	GeminiModel              = Key{Name: "GEMINI_MODEL", Def: "gemini-3.5-flash-lite"}
	GrokEnabled              = boolKey("GROK_ENABLED", true)
	GrokRateLimit            = intKey("GROK_RATE_LIMIT", 3, 1, 1000) // requests per user per minute
	GrokMaxChars             = intKey("GROK_MAX_CHARS", 3000, 100, 50000)
	GrokPrompt               = Key{Name: "GROK_PROMPT", Def: defaultGrokPrompt}
	GrokRetellPrompt         = Key{Name: "GROK_RETELL_PROMPT", Def: defaultGrokRetellPrompt}
	GrokOpinionPrompt        = Key{Name: "GROK_OPINION_PROMPT", Def: defaultGrokOpinionPrompt}
	ScraperUserAgents        = Key{Name: "SCRAPER_USER_AGENTS", Def: strings.Join(config.DefaultScraperUserAgents, "\n"), Validate: validateUserAgents}
)

// Keys read through the Provider but not edited in the settings panel.
var (
	BotToken      = Key{Name: "BOT_TOKEN"}
	AdminUsername = Key{Name: "ADMIN_USERNAME"}
	AdminChatID   = Key{Name: "ADMIN_CHAT_ID"} // learned from the admin's first private message
)

// Editable lists the keys managed by the web settings panel, in display order.
var Editable = []Key{
	DownloadConcurrency, DownloadRetentionHours, TGFileLimitMB, ClipMaxDurationSeconds, ClipMaxDurationHQSeconds,
	YTDLPProxy, ClipCRF, SubsCRF, ClipPreset, ClipAudioBitrate, ClipDeleteStatus,
	TranslationTimeout, SubsYouTubeOnly, SubsSourcePrefRU, TranslationPrompt, GeminiAPIKey, GeminiModel,
	GrokEnabled, GrokRateLimit, GrokMaxChars, GrokPrompt, GrokRetellPrompt, GrokOpinionPrompt,
	ScraperUserAgents,
}

// Lookup returns the editable key with the given name.
func Lookup(name string) (Key, bool) {
	i := slices.IndexFunc(Editable, func(k Key) bool { return k.Name == name })
	if i < 0 {
		return Key{}, false
	}
	return Editable[i], true
}

func validateBitrate(v string) error {
	if !bitrateRe.MatchString(strings.TrimSpace(v)) {
		return fmt.Errorf("must look like 192k")
	}
	return nil
}

// validateUserAgents requires a non-comment line, matching what scraper.ParseUserAgents accepts.
func validateUserAgents(v string) error {
	for _, line := range strings.Split(v, "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			return nil
		}
	}
	return fmt.Errorf("add at least one user agent line")
}
