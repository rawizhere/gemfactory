package handlers

import (
	"context"
	"html"
	"strconv"
	"strings"
	"time"

	"github.com/mymmrac/telego"
	"go.uber.org/zap"

	"gemfactory/internal/middleware"
	"gemfactory/internal/translate"
)

type GrokHandlers struct {
	*BaseHandler
	limiter *middleware.GrokLimiter
}

func NewGrokHandlers(base *BaseHandler) *GrokHandlers {
	return &GrokHandlers{
		BaseHandler: base,
		limiter:     middleware.NewGrokLimiter(),
	}
}

func (h *GrokHandlers) FactCheck(ctx context.Context, message *telego.Message) {
	if message == nil || message.ReplyToMessage == nil {
		return
	}

	enabledStr, err := h.Services.Config.Get(ctx, "GROK_ENABLED")
	if err == nil && !translate.IsTruthy(enabledStr) {
		return
	}

	isAdmin := message.From != nil && (message.From.Username == h.Config.AdminUsername || h.IsAdmin(message.From))
	if !isAdmin {
		rateLimit := 3
		if rlStr, err := h.Services.Config.Get(ctx, "GROK_RATE_LIMIT"); err == nil && rlStr != "" {
			if v, err := strconv.Atoi(rlStr); err == nil && v > 0 {
				rateLimit = v
			}
		}

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

	replyMsg := message.ReplyToMessage
	replyText := replyMsg.Text
	if replyText == "" {
		replyText = replyMsg.Caption
	}

	var targetText string
	if message.Quote != nil && strings.TrimSpace(message.Quote.Text) != "" {
		targetText = "Проверяемая цитата:\n\"" + message.Quote.Text + "\"\n\nКонтекст исходного сообщения:\n\"" + replyText + "\""
	} else {
		targetText = "Текст для проверки:\n\"" + replyText + "\""
	}

	if strings.TrimSpace(replyText) == "" && (message.Quote == nil || strings.TrimSpace(message.Quote.Text) == "") {
		return
	}

	maxChars := 3000
	if mcStr, err := h.Services.Config.Get(ctx, "GROK_MAX_CHARS"); err == nil && mcStr != "" {
		if v, err := strconv.Atoi(mcStr); err == nil && v > 0 {
			maxChars = v
		}
	}

	runes := []rune(targetText)
	if len(runes) > maxChars {
		targetText = string(runes[:maxChars])
	}
	if strings.TrimSpace(targetText) == "" {
		return
	}

	prompt, err := h.Services.Config.Get(ctx, "GROK_PROMPT")
	if err != nil || strings.TrimSpace(prompt) == "" {
		prompt = translate.DefaultGrokPrompt
	}

	cfg := translate.ResolveConfig(func(k string) (string, bool) {
		if h.Services == nil || h.Services.Config == nil {
			return "", false
		}
		v, err := h.Services.Config.Get(ctx, k)
		if err != nil || strings.TrimSpace(v) == "" {
			return "", false
		}
		return strings.TrimSpace(v), true
	})

	resp, _, err := translate.Complete(ctx, prompt, targetText, cfg)
	if err != nil {
		h.Logger.Error("grok factcheck failed", zap.Error(err))
		return
	}

	_, _ = h.TG.SendMessageRaw(ctx, message.Chat.ID, html.EscapeString(resp), message.MessageID)
}
