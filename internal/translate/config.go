// Package translate carries subtitle translation: provider clients, the fallback chain and output validation.
package translate

import (
	"os"
	"strings"
	"time"
)

const (
	ProviderGoogle     = "google"
	ProviderGemini     = "gemini"
	ProviderGroq       = "groq"
	ProviderOpencode   = "opencode"
	ProviderNvidia     = "nvidia"
	ProviderOpenRouter = "openrouter"
)

// DefaultOpencodeModels is the free-model fallback chain on OpenCode Zen.
var DefaultOpencodeModels = []string{
	"laguna-s-2.1-free",
	"nemotron-3.5-lightning-free",
	"hy3-free",
}

// DefaultNvidiaModels is the free-model chain on NVIDIA NIM.
var DefaultNvidiaModels = []string{
	"minimaxai/minimax-m3",
	"openai/gpt-oss-120b",
	"meta/muse-glimmer-30b",
	"stepfun-ai/step-3.7-flash",
}

// DefaultOpenRouterModels is the free-model chain on OpenRouter.
var DefaultOpenRouterModels = []string{
	"minimax/minimax-m3:free",
	"deepseek/deepseek-v4-flash-0731",
	"nvidia/nemotron-3.5-lightning:free",
	"dots-studio/dots-3-note-preview:free",
}

const translatorRole = "You are a professional video subtitle translator."

const DefaultTranslationPrompt = `Translate subtitles for entertainment videos: variety shows, vlogs, livestreams, song lyrics. The source language varies — detect it from the lines themselves.
1. Meaning over words: natural spoken register matching the original's energy; never wooden literalism or calques.
2. Names: personal, stage and group names, fandoms and products are names — transliterate them by sound into the target script; if translating a token yields a common word, it is a name. Acronyms (MBTI, TMI, PPL) and stylized brand/model names stay in Latin script.
3. Formatting: keep speaker tags ([ALL], Host:) and multi-speaker dash structure intact; translate bracketed notes naturally inside the brackets; preserve ♪, emojis, tildes, asterisks and Asian brackets.
4. Song lyrics: translate the thought and rhythm, not word-by-word.
5. Silently fix obvious ASR/speech-recognition errors from context.`

const ruAddendum = `For Russian: informal "ты", lively youth phrasing; transliterate Korean honorifics (онни, оппа, макнэ, хён); names transliterate by sound (Liv -> Лив, May -> Мэй), a common noun next to a name stays a common word (리브 미모 = красота Лив, not "Liv Mimo"); bracketed notes translate naturally ([Laughter] -> [Смех]).`

// DefaultSourcePrefRU is the fallback source-language order for ru subtitles.
const DefaultSourcePrefRU = "en,ko"

// BuildSystemInstruction combines the configured prompt with target-language rules.
func BuildSystemInstruction(prompt, targetLang string) string {
	s := prompt
	if targetLang == "ru" {
		if s != "" {
			s += "\n"
		}
		s += ruAddendum
	}
	return s
}

// Config holds the provider fallback order, API keys, models, custom prompt and LLM request timeout.
type Config struct {
	GoogleOnly       bool // force Google Translate, skip all LLM providers
	GeminiKey        string
	GroqKey          string
	OpencodeKey      string
	NvidiaKey        string
	OpenRouterKey    string
	GeminiModels     []string
	GroqModels       []string
	OpencodeModels   []string
	NvidiaModels     []string
	OpenRouterModels []string
	FallbackOrder    []string
	Prompt           string
	SourcePrefRU     []string // preferred source languages when translating into ru
	Timeout          time.Duration
}

var DefaultGeminiModels = []string{
	"gemini-3.7-flash",
	"gemini-3.5-flash-lite",
	"gemini-3.1-flash-lite",
	"gemini-2.5-flash-lite",
	"gemini-2.5-flash",
	"gemini-flash-latest",
}

var DefaultGroqModels = []string{
	"openai/gpt-oss-120b",
}

func ParseCSV(s string) []string {
	parts := strings.Split(s, ",")
	var res []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			res = append(res, p)
		}
	}
	return res
}

func IsTruthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// DefaultConfig resolves the translation config from env with built-in defaults.
func DefaultConfig() Config {
	cfg := Config{
		GeminiKey:        strings.TrimSpace(os.Getenv("GEMINI_API_KEY")),
		GroqKey:          strings.TrimSpace(os.Getenv("GROQ_API_KEY")),
		OpencodeKey:      strings.TrimSpace(os.Getenv("OPENCODE_API_KEY")),
		NvidiaKey:        strings.TrimSpace(os.Getenv("NVIDIA_API_KEY")),
		OpenRouterKey:    strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")),
		GeminiModels:     ParseCSV(os.Getenv("GEMINI_MODELS")),
		GroqModels:       ParseCSV(os.Getenv("GROQ_MODELS")),
		OpencodeModels:   ParseCSV(os.Getenv("OPENCODE_MODELS")),
		NvidiaModels:     ParseCSV(os.Getenv("NVIDIA_MODELS")),
		OpenRouterModels: ParseCSV(os.Getenv("OPENROUTER_MODELS")),
		FallbackOrder:    ParseCSV(os.Getenv("TRANSLATION_FALLBACK_ORDER")),
		Prompt:           strings.TrimSpace(os.Getenv("TRANSLATION_PROMPT")),
		SourcePrefRU:     ParseCSV(os.Getenv("SUBS_SOURCE_PREF_RU")),
	}
	cfg.GoogleOnly = IsTruthy(os.Getenv("SUBS_GOOGLE_ONLY"))
	return ApplyDefaults(cfg)
}

func ApplyDefaults(cfg Config) Config {
	if len(cfg.GeminiModels) == 0 {
		cfg.GeminiModels = DefaultGeminiModels
	}
	if len(cfg.GroqModels) == 0 {
		cfg.GroqModels = DefaultGroqModels
	}
	if len(cfg.OpencodeModels) == 0 {
		cfg.OpencodeModels = DefaultOpencodeModels
	}
	if len(cfg.NvidiaModels) == 0 {
		cfg.NvidiaModels = DefaultNvidiaModels
	}
	if len(cfg.OpenRouterModels) == 0 {
		cfg.OpenRouterModels = DefaultOpenRouterModels
	}
	if len(cfg.SourcePrefRU) == 0 {
		cfg.SourcePrefRU = ParseCSV(DefaultSourcePrefRU)
	}
	if cfg.Prompt == "" {
		cfg.Prompt = DefaultTranslationPrompt
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout()
	}
	return cfg
}

// defaultTimeout bounds a single LLM request; a whole subtitle file goes out in one call.
func defaultTimeout() time.Duration {
	if v, err := time.ParseDuration(os.Getenv("TRANSLATION_TIMEOUT")); err == nil && v > 0 {
		return v
	}
	return 180 * time.Second
}

func BuildFallbackChain(cfg Config) []string {
	if cfg.GoogleOnly {
		return []string{ProviderGoogle}
	}
	if len(cfg.FallbackOrder) > 0 {
		var chain []string
		for _, p := range cfg.FallbackOrder {
			p = strings.ToLower(strings.TrimSpace(p))
			if p == ProviderGoogle || p == ProviderGemini || p == ProviderGroq || p == ProviderOpencode || p == ProviderNvidia || p == ProviderOpenRouter {
				chain = append(chain, p)
			}
		}
		if len(chain) > 0 {
			return chain
		}
	}
	return []string{ProviderOpenRouter, ProviderOpencode, ProviderNvidia, ProviderGroq}
}
