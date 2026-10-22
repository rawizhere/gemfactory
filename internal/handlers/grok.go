package handlers

import (
	"context"
	"html"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/mymmrac/telego"
	"go.uber.org/zap"

	"gemfactory/internal/middleware"
	"gemfactory/internal/translate"
)

// GrokMode is one @grok command: which prompt to load, how to frame the input text and how to call the model.
type GrokMode struct {
	promptKey   string
	defPrompt   string
	inputLabel  string
	quoteLabel  string
	temperature float32
	search      bool // ground the answer in Google Search while GROK_SEARCH is on
}

var (
	GrokFactCheck = GrokMode{"GROK_PROMPT", translate.DefaultGrokPrompt, "Текст для проверки", "Проверяемая цитата", 0.3, true}
	GrokRetell    = GrokMode{"GROK_RETELL_PROMPT", translate.DefaultGrokRetellPrompt, "Текст для пересказа", "Пересказываемая цитата", 0.5, false}
	GrokOpinion   = GrokMode{"GROK_OPINION_PROMPT", translate.DefaultGrokOpinionPrompt, "Текст для оценки", "Цитата для оценки", 0.7, false}
)

// grokFailReplies answer a failed model call in the prompts' dry, emoji-free voice.
var grokFailReplies = []string{
	"Что-то я завис. Спроси попозже.",
	"Мысль была, но потерялась по дороге.",
	"Сервер молчит, а без него я так себе собеседник.",
	"Не сейчас. Даже у меня бывают выходные.",
}

type GrokHandlers struct {
	*BaseHandler
	limiter *middleware.Limiter
}

func NewGrokHandlers(base *BaseHandler) *GrokHandlers {
	return &GrokHandlers{
		BaseHandler: base,
		limiter:     middleware.NewLimiter(),
	}
}

// Run answers an @grok reply; question is the user's text after the command and may be empty.
func (h *GrokHandlers) Run(ctx context.Context, message *telego.Message, mode GrokMode, question string) {
	if message == nil || message.ReplyToMessage == nil {
		return
	}

	if !h.Services.Settings.Bool(ctx, "GROK_ENABLED", true) {
		return
	}

	replyMsg := message.ReplyToMessage
	replyText := replyMsg.Text
	if replyText == "" {
		replyText = replyMsg.Caption
	}
	quote := ""
	if message.Quote != nil {
		quote = message.Quote.Text
	}
	if strings.TrimSpace(replyText) == "" && strings.TrimSpace(quote) == "" {
		_, _ = h.TG.SendMessageRaw(ctx, message.Chat.ID, "не коменчу фотки", replyMsg.MessageID)
		return
	}

	if !h.IsAdmin(message.From) {
		rateLimit := h.Services.Settings.Int(ctx, "GROK_RATE_LIMIT", translate.DefaultGrokRateLimit)

		userID := int64(0)
		if message.From != nil {
			userID = message.From.ID
		}

		allowed, shouldNotify := h.limiter.Check(userID, rateLimit, time.Minute)
		if !allowed {
			if shouldNotify {
				_, _ = h.TG.SendMessageRaw(ctx, message.Chat.ID, "Мне лень", message.MessageID)
			}
			return
		}
	}

	maxChars := h.Services.Settings.Int(ctx, "GROK_MAX_CHARS", translate.DefaultGrokMaxChars)
	replyText = truncateRunes(replyText, maxChars)

	var targetText string
	if strings.TrimSpace(quote) != "" {
		targetText = mode.quoteLabel + ":\n\"" + truncateRunes(quote, maxChars) + "\"\n\nКонтекст исходного сообщения:\n\"" + replyText + "\""
	} else {
		targetText = mode.inputLabel + ":\n\"" + replyText + "\""
	}
	if question != "" {
		targetText += "\n\nВопрос пользователя: " + truncateRunes(question, maxChars)
	}

	prompt := h.Services.Settings.Value(ctx, mode.promptKey, mode.defPrompt)
	cfg := translate.ResolveConfig(ctx, h.Services.Settings)

	h.TG.SendTyping(ctx, message.Chat.ID, message.MessageThreadID)

	search := mode.search && h.Services.Settings.Bool(ctx, "GROK_SEARCH", true)

	resp, err := translate.Complete(ctx, cfg, prompt, targetText, mode.temperature, search)
	if err != nil {
		h.Logger.Error("grok request failed", zap.Error(err), zap.String("mode", mode.promptKey))
		// The handler timeout may have fired; the apology still goes out.
		sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_, _ = h.TG.SendMessageRaw(sendCtx, message.Chat.ID, grokFailReplies[rand.N(len(grokFailReplies))], message.MessageID)
		return
	}

	_, _ = h.TG.SendMessageRaw(ctx, message.Chat.ID, html.EscapeString(resp), replyMsg.MessageID)
}

func truncateRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
