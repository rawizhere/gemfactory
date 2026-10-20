// Package translate carries subtitle translation: provider clients, the fallback chain and output validation.
package translate

import (
	"os"
	"strconv"
	"strings"
	"time"

	"gemfactory/internal/settings"
)

const (
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

// DefaultOpenRouterModels is the fallback model chain on OpenRouter.
var DefaultOpenRouterModels = []string{
	"google/gemini-2.5-flash-lite",
	"mistralai/mistral-small-24b-instruct-2501",
	"meta-llama/llama-3.3-70b-instruct",
	"minimax/minimax-m3:free",
}

const translatorRole = "You are a professional video subtitle translator."

// DefaultTranslationPrompt is the fallback when TRANSLATION_PROMPT is empty; the live prompt is edited in the web panel.
// Output-format rules live in the per-provider user message, so editing the prompt cannot break parsing.
const DefaultTranslationPrompt = `Target: Russian subtitles for K-pop variety shows, vlogs, livestreams and song lyrics.

1. Lines and dialogue
- Each numbered line is one on-screen subtitle. A sentence often continues on the next line: read the neighbours, then translate so the lines read naturally in order, keeping each line's content under its own number. Do not merge, split, move or drop lines.
- "- A - B" in one line is two speakers: keep both dashes in that same line, keep their phrases separate, no line breaks.
- A speaker tag at the start of a line ([ALL], [WONYOUNG], Host:) is copied unchanged.
- Any other bracketed text is an editor's note: translate it inside the same brackets ([Laughter] -> [Смех], (Sighs) -> (Вздыхает), [Music] -> [Музыка]).
- Copy ♪, ♫, emojis, ~, *, 「」, 【】, 《》 and quotes unchanged and in place.

2. Names
- People (idols, members, hosts, guests): Cyrillic by sound, spelled the same in every line.
  Examples: NAYEON -> Наён, WONYOUNG -> Вонён, CHAEWON -> Чэвон, YUJIN -> Юджин, JI-HYO -> Джихё, JAE-SUK -> Джэсок, SO-MIN -> Сомин, YE-ON -> Йеон, YEEUN -> Йеын, Winter -> Винтер, Joy -> Джой.
  Sounds: eo -> о, eu -> ы, ae -> э, yeo/yo -> ё, final ng -> н, j -> дж, ch -> ч.
  Surnames: Kim -> Ким, Lee/Yi -> Ли, Park -> Пак, Choi -> Чхве, Jung/Jeong -> Чон, Kang -> Кан, Cho -> Чо, Yoon -> Юн, Jang -> Чан, Lim -> Им, Han -> Хан, Shin -> Син, Song -> Сон, Hwang -> Хван.
- A hyphenated or all-caps name (YE-ON, JI-HYO) is one name, never split into syllables or translated as words.
- Groups, fandoms, shows and brands stay in Latin as written: RESCENE, aespa, TWICE, ONCE.
- If a word would translate into an ordinary word but stands where a name fits, it is a name.
- Acronyms stay in Latin: MBTI, TMI, OOTD, PPL, ASMR.
- Honorifics in Cyrillic: онни, оппа, нуна, хён, макнэ, сонбэ(ним); "-ssi" -> "-сси".

3. Style
- Lively spoken Russian with the energy of the original; translate idioms, slang and jokes by meaning, not word-for-word.
- "ты" between members, friends and hosts; the audience is "вы" / "ребята".
- Short lines a viewer reads in 2–3 seconds; drop filler words instead of padding.
- Korean interjections get Russian equivalents: 대박 -> Офигеть, 헐 -> Чего?!, 아이고 -> Ой-ой, 화이팅 -> Вперёд!
- Mixed Korean and English in one line becomes one natural Russian sentence.
- Lyrics: meaning and mood in short lines, no forced rhyme.
- Keep swearing at the same strength; do not censor, soften or comment.
- Silently fix obvious typos and speech-recognition errors.

4. Example
[WONYOUNG] Wait, I really didn't -> [WONYOUNG] Стоп, я правда совсем
expect that at all -> такого не ожидала
- Are you serious? - Yes! -> - Ты серьёзно? - Ага!
[Laughter] 대박 ♪ -> [Смех] Офигеть ♪`

// Grok defaults used when GROK_RATE_LIMIT / GROK_MAX_CHARS are unset; the web panel shows the same values.
const (
	DefaultGrokRateLimit = 3    // requests per user per minute
	DefaultGrokMaxChars  = 3000 // input runes sent to the model
)

const DefaultGrokPrompt = `Ты — Grok в Telegram. Твоя задача — ответить на вопрос пользователя «это правда?», проверив утверждение или цитату.

Правила:
1. Отвечай прямо, естественно и по делу, как живой собеседник с острым умом и сухим юмором. Никаких шаблонных плашек, оценок в баллах и искусственных форматов.
2. Сразу говори суть: правда это, чушь, вырванный из контекста кусок или просто чье-то субъективное мнение.
3. Опирайся строго на реальные факты и логику. Никогда ничего не выдумывай.
4. Будь лаконичным (1–3 предложения), без воды и морализаторства.
5. Не используй абсолютно никаких эмодзи.`

const DefaultGrokRetellPrompt = `Ты — Grok в Telegram. Тебя попросили пересказать текст, на который ответили этой фразой.

Правила:
1. Передай главную мысль своими словами: что случилось, кто и что сказал, чем кончилось.
2. Держись фактов текста. Не додумывай и не добавляй контекста, которого там нет.
3. Сохраняй интонацию автора: если текст спорный или агрессивный, пересказывай спорность, а не свою версию.
4. Лаконично (1–3 предложения). Никаких «в данном тексте автор поднимает тему».
5. Не используй абсолютно никаких эмодзи.`

const DefaultGrokOpinionPrompt = `Ты — Grok в Telegram. У тебя спросили мнение о тексте, на который ответили этой фразой.

Правила:
1. Отвечай как живой собеседник с острым умом и сухим юмором: прямо, субъективно, со своей позицией.
2. Сразу verdict — согласен или нет, нравится или нет, и почему. Одного-двух предложений достаточно.
3. Опирайся на факты и логику. Если для мнения не хватает данных, так и скажи.
4. Не читай лекций, не морализируй, не проговаривай оговорки про то, что ты ИИ.
5. Лаконично (1–3 предложения). Не используй абсолютно никаких эмодзи.`

// DefaultSourcePrefRU is the fallback source-language order for ru subtitles.
const DefaultSourcePrefRU = "en,ko"

// Config holds the provider fallback order, API keys, models, custom prompt and LLM request timeout.
type Config struct {
	YouTubeOnly      bool // use YouTube auto-translate, skip all LLM providers
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
	"gemini-3.5-flash-lite",
	"gemini-3.1-flash-lite",
	"gemini-3.5-flash",
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
	cfg.YouTubeOnly = settings.IsTruthy(os.Getenv("SUBS_YOUTUBE_ONLY"))
	return ApplyDefaults(cfg)
}

// ResolveConfig layers DB-backed settings on top of env defaults.
func ResolveConfig(getter func(string) (string, bool)) Config {
	cfg := DefaultConfig()
	if getter == nil {
		return cfg
	}
	if v, ok := getter("GEMINI_API_KEY"); ok {
		cfg.GeminiKey = v
	}
	if v, ok := getter("GROQ_API_KEY"); ok {
		cfg.GroqKey = v
	}
	if v, ok := getter("OPENCODE_API_KEY"); ok {
		cfg.OpencodeKey = v
	}
	if v, ok := getter("NVIDIA_API_KEY"); ok {
		cfg.NvidiaKey = v
	}
	if v, ok := getter("OPENROUTER_API_KEY"); ok {
		cfg.OpenRouterKey = v
	}
	if v, ok := getter("GEMINI_MODELS"); ok {
		cfg.GeminiModels = ParseCSV(v)
	}
	if v, ok := getter("GROQ_MODELS"); ok {
		cfg.GroqModels = ParseCSV(v)
	}
	if v, ok := getter("OPENCODE_MODELS"); ok {
		cfg.OpencodeModels = ParseCSV(v)
	}
	if v, ok := getter("NVIDIA_MODELS"); ok {
		cfg.NvidiaModels = ParseCSV(v)
	}
	if v, ok := getter("OPENROUTER_MODELS"); ok {
		cfg.OpenRouterModels = ParseCSV(v)
	}
	if v, ok := getter("TRANSLATION_FALLBACK_ORDER"); ok {
		cfg.FallbackOrder = ParseCSV(v)
	}
	if v, ok := getter("TRANSLATION_PROMPT"); ok {
		cfg.Prompt = v
	}
	if v, ok := getter("SUBS_SOURCE_PREF_RU"); ok {
		cfg.SourcePrefRU = ParseCSV(v)
	}
	if v, ok := getter("SUBS_YOUTUBE_ONLY"); ok {
		cfg.YouTubeOnly = settings.IsTruthy(v)
	}
	if v, ok := getter("TRANSLATION_TIMEOUT"); ok {
		if sec, err := strconv.Atoi(v); err == nil && sec > 0 {
			cfg.Timeout = time.Duration(sec) * time.Second
		}
	}
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
	if len(cfg.FallbackOrder) > 0 {
		var chain []string
		for _, p := range cfg.FallbackOrder {
			p = strings.ToLower(strings.TrimSpace(p))
			if p == ProviderGemini || p == ProviderGroq || p == ProviderOpencode || p == ProviderNvidia || p == ProviderOpenRouter {
				chain = append(chain, p)
			}
		}
		if len(chain) > 0 {
			return chain
		}
	}
	return []string{ProviderGemini, ProviderOpenRouter, ProviderOpencode, ProviderNvidia, ProviderGroq}
}
