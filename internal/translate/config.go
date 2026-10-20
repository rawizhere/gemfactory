// Package translate carries subtitle translation: provider clients, the fallback chain and output validation.
package translate

import (
	"context"
	"slices"
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

// provider describes one translation backend; baseURL is empty for Gemini, which uses its own SDK.
type provider struct {
	baseURL  string
	defaults []string
}

var providers = map[string]provider{
	ProviderGemini:     {"", DefaultGeminiModels},
	ProviderGroq:       {"https://api.groq.com/openai/v1", DefaultGroqModels},
	ProviderOpencode:   {"https://opencode.ai/zen/v1", DefaultOpencodeModels},
	ProviderNvidia:     {"https://integrate.api.nvidia.com/v1", DefaultNvidiaModels},
	ProviderOpenRouter: {"https://openrouter.ai/api/v1", DefaultOpenRouterModels},
}

// providerOrder is the default fallback chain.
var providerOrder = []string{ProviderGemini, ProviderOpenRouter, ProviderOpencode, ProviderNvidia, ProviderGroq}

// modelsFor returns the configured model chain for p, or its built-in default.
func (c Config) modelsFor(p string) []string {
	if m := c.Models[p]; len(m) > 0 {
		return m
	}
	return providers[p].defaults
}

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
	YouTubeOnly   bool                // use YouTube auto-translate, skip all LLM providers
	Keys          map[string]string   // provider -> API key
	Models        map[string][]string // provider -> model chain
	FallbackOrder []string
	Prompt        string
	SourcePrefRU  []string // preferred source languages when translating into ru
	Timeout       time.Duration
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

// ResolveConfig reads the translation config with the DB -> env -> default precedence.
func ResolveConfig(ctx context.Context, s settings.Provider) Config {
	cfg := Config{
		YouTubeOnly:   s.Bool(ctx, "SUBS_YOUTUBE_ONLY", false),
		Keys:          map[string]string{},
		Models:        map[string][]string{},
		FallbackOrder: ParseCSV(s.Value(ctx, "TRANSLATION_FALLBACK_ORDER", "")),
		Prompt:        s.Value(ctx, "TRANSLATION_PROMPT", DefaultTranslationPrompt),
		SourcePrefRU:  ParseCSV(s.Value(ctx, "SUBS_SOURCE_PREF_RU", DefaultSourcePrefRU)),
		Timeout:       parseTimeout(s.Value(ctx, "TRANSLATION_TIMEOUT", "")),
	}
	for _, p := range providerOrder {
		env := strings.ToUpper(p)
		cfg.Keys[p] = s.Value(ctx, env+"_API_KEY", "")
		cfg.Models[p] = ParseCSV(s.Value(ctx, env+"_MODELS", strings.Join(providers[p].defaults, ",")))
	}
	return cfg
}

const defaultTimeout = 180 * time.Second

// parseTimeout accepts seconds (web panel) or a Go duration (env, e.g. "3m").
func parseTimeout(v string) time.Duration {
	if sec, err := strconv.Atoi(v); err == nil && sec > 0 {
		return time.Duration(sec) * time.Second
	}
	if d, err := time.ParseDuration(v); err == nil && d > 0 {
		return d
	}
	return defaultTimeout
}

func BuildFallbackChain(cfg Config) []string {
	var chain []string
	for _, p := range cfg.FallbackOrder {
		p = strings.ToLower(strings.TrimSpace(p))
		if _, ok := providers[p]; ok {
			chain = append(chain, p)
		}
	}
	if len(chain) > 0 {
		return chain
	}
	return AllProviders()
}

// AllProviders lists every provider in default fallback order.
func AllProviders() []string {
	return slices.Clone(providerOrder)
}

// IsProvider reports whether p names a known provider.
func IsProvider(p string) bool {
	_, ok := providers[p]
	return ok
}
