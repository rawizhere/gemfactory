// Package app handles command and callback routing for incoming Telegram updates.
package app

import (
	"context"
	"strconv"
	"strings"
	"unicode"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	"go.uber.org/zap"

	"gemfactory/internal/config"
	"gemfactory/internal/downloader"
	"gemfactory/internal/handlers"
	"gemfactory/internal/keyboard"
	"gemfactory/internal/middleware"
	"gemfactory/internal/notify"
	"gemfactory/internal/service"
	"gemfactory/internal/telegram"
)

// Router dispatches telegram updates to handler methods through middleware.
type Router struct {
	handlers *handlers.Handlers
	config   *config.Config
	services *service.Services
	logger   *zap.Logger
}

func NewRouter(services *service.Services, config *config.Config, keyboard *keyboard.Manager, logger *zap.Logger, tg *telegram.Client, downloads *downloader.Service) *Router {
	return &Router{
		handlers: handlers.New(services, config, keyboard, logger, tg, downloads),
		config:   config,
		services: services,
		logger:   logger,
	}
}

// RegisterRoutes wires command and callback handlers; only the first matching route runs.
// Handlers added via Use must call ctx.Next(update), otherwise the chain stops silently.
func (r *Router) RegisterRoutes(bh *th.BotHandler) {
	bh.Use(middleware.Handlers(r.logger)...)
	bh.Use(r.rememberAdmin())

	commandRoutes := []struct {
		command string
		handler th.MessageHandler
	}{
		{"start", func(ctx *th.Context, m telego.Message) error { r.handlers.User.Start(ctx, &m); return nil }},
		{"month", func(ctx *th.Context, m telego.Message) error { r.handlers.User.Month(ctx, &m); return nil }},
		{"search", func(ctx *th.Context, m telego.Message) error { r.handlers.User.Search(ctx, &m); return nil }},
		{"artists", func(ctx *th.Context, m telego.Message) error { r.handlers.User.Artists(ctx, &m); return nil }},
		{"metrics", func(ctx *th.Context, m telego.Message) error { r.handlers.User.Metrics(ctx, &m); return nil }},
		{"admin", func(ctx *th.Context, m telego.Message) error { r.handlers.Admin.Admin(ctx, &m); return nil }},
		{"add_artist", func(ctx *th.Context, m telego.Message) error { r.handlers.Admin.AddArtist(ctx, &m); return nil }},
		{"remove_artist", func(ctx *th.Context, m telego.Message) error { r.handlers.Admin.RemoveArtist(ctx, &m); return nil }},
		{"export", func(ctx *th.Context, m telego.Message) error { r.handlers.Admin.Export(ctx, &m); return nil }},
		{"parse", func(ctx *th.Context, m telego.Message) error { r.handlers.Admin.Parse(ctx, &m); return nil }},
		{"clip", clipHandler(r.handlers, "clip")},
		{"gif", clipHandler(r.handlers, "gif")},
		{"subs", clipHandler(r.handlers, "subs")},
		{"mp3", clipHandler(r.handlers, "mp3")},
	}
	for _, route := range commandRoutes {
		bh.HandleMessage(route.handler, th.CommandEqual(route.command))
	}

	bh.HandleMessage(func(ctx *th.Context, m telego.Message) error {
		if r.isClipHelpTopic(&m) {
			r.handlers.Clip.Help(ctx, &m)
		} else {
			r.handlers.User.Help(ctx, &m)
		}
		return nil
	}, th.CommandEqual("help"))

	bh.HandleMessage(func(ctx *th.Context, m telego.Message) error {
		if mode, question, ok := detectGrokMode(&m); ok {
			r.handlers.Grok.Run(ctx, &m, mode, question)
		}
		return nil
	}, grokPredicate())

	// Implicit clip messages (URL + timecodes, no command) must win over the plain URL route.
	bh.HandleMessage(func(ctx *th.Context, m telego.Message) error {
		r.handlers.Clip.Implicit(ctx, &m)
		return nil
	}, messageWithImplicitClip())

	// Non-command messages containing a URL start a direct download; registered last so commands win.
	bh.HandleMessage(func(ctx *th.Context, m telego.Message) error {
		r.handlers.Clip.DirectLink(ctx, &m, downloader.ExtractFirstURL(m.Text))
		return nil
	}, messageWithURL())

	kb := r.handlers.User.Keyboard
	callbackRoutes := []struct {
		predicate th.Predicate
		handler   th.CallbackQueryHandler
	}{
		{th.CallbackDataPrefix("month_"), func(ctx *th.Context, q telego.CallbackQuery) error {
			return kb.HandleMonthCallback(ctx, &q)
		}},
		{th.CallbackDataEqual("show_all_months"), func(ctx *th.Context, q telego.CallbackQuery) error {
			return kb.HandleAllMonthsCallback(ctx, &q)
		}},
		{th.CallbackDataEqual("back_to_main"), func(ctx *th.Context, q telego.CallbackQuery) error {
			return kb.HandleMainCallback(ctx, &q)
		}},
	}
	for _, route := range callbackRoutes {
		bh.HandleCallbackQuery(route.handler, route.predicate)
	}
}

// rememberAdmin stores the admin chat id from private admin messages, then continues the chain.
func (r *Router) rememberAdmin() th.Handler {
	return func(ctx *th.Context, update telego.Update) error {
		m := update.Message
		if m == nil || m.From == nil || m.From.Username == "" || m.Chat.Type != "private" {
			return ctx.Next(update)
		}
		admin := strings.TrimPrefix(r.config.AdminUsername, "@")
		if !strings.EqualFold(m.From.Username, admin) {
			return ctx.Next(update)
		}
		chatID := strconv.FormatInt(m.Chat.ID, 10)
		if stored, err := r.services.Configs.Get(ctx.Context(), notify.AdminChatIDKey); err != nil || stored == nil || stored.Value != chatID {
			if err := r.services.Configs.Set(ctx.Context(), notify.AdminChatIDKey, chatID); err != nil {
				r.logger.Warn("Failed to store admin chat id", zap.Error(err))
			}
		}
		return ctx.Next(update)
	}
}

func clipHandler(h *handlers.Handlers, command string) th.MessageHandler {
	return func(ctx *th.Context, m telego.Message) error {
		switch command {
		case "clip":
			h.Clip.Clip(ctx, &m)
		case "gif":
			h.Clip.Gif(ctx, &m)
		case "subs":
			h.Clip.Subs(ctx, &m)
		case "mp3":
			h.Clip.MP3(ctx, &m)
		}
		return nil
	}
}

func grokPredicate() th.Predicate {
	return func(_ context.Context, update telego.Update) bool {
		_, _, ok := detectGrokMode(update.Message)
		return ok
	}
}

// grokPunct separates words in an @grok command; each byte maps to one space, so offsets are kept.
var grokPunct = strings.NewReplacer(",", " ", ":", " ", "?", " ", "!", " ", ";", " ")

// detectGrokMode matches an @grok reply against the known commands; factcheck is tested first.
// A bare "@grok" is a factcheck. question is the user's text after the command.
func detectGrokMode(m *telego.Message) (mode handlers.GrokMode, question string, ok bool) {
	if m == nil || m.ReplyToMessage == nil || m.Text == "" {
		return handlers.GrokMode{}, "", false
	}
	words, ends := splitWords(grokPunct.Replace(m.Text), 4)
	if len(words) == 0 || words[0] != "@grok" {
		return handlers.GrokMode{}, "", false
	}
	rest := func(n int) string {
		return strings.TrimSpace(strings.TrimLeft(m.Text[ends[n-1]:], " ,:;?!"))
	}

	switch {
	case len(words) == 1:
		return handlers.GrokFactCheck, "", true
	case len(words) >= 3 && words[1] == "это" && words[2] == "правда":
		return handlers.GrokFactCheck, rest(3), true
	case len(words) >= 4 && words[1] == "is" && words[2] == "this" && words[3] == "true":
		return handlers.GrokFactCheck, rest(4), true
	case strings.HasPrefix(words[1], "переска"),
		words[1] == "retell", words[1] == "summarize", words[1] == "summary", words[1] == "tldr":
		return handlers.GrokRetell, rest(2), true
	case strings.HasPrefix(words[1], "мнен"),
		words[1] == "opinion", words[1] == "thoughts", words[1] == "think", words[1] == "take":
		return handlers.GrokOpinion, rest(2), true
	}

	return handlers.GrokMode{}, "", false
}

// splitWords returns up to limit lowercased whitespace-separated words of s and the byte offset after each.
func splitWords(s string, limit int) (words []string, ends []int) {
	off := 0
	for len(words) < limit {
		trimmed := strings.TrimLeftFunc(s[off:], unicode.IsSpace)
		off = len(s) - len(trimmed)
		if trimmed == "" {
			break
		}
		n := strings.IndexFunc(trimmed, unicode.IsSpace)
		if n < 0 {
			n = len(trimmed)
		}
		words = append(words, strings.ToLower(trimmed[:n]))
		off += n
		ends = append(ends, off)
	}
	return words, ends
}

// messageWithURL matches non-command text messages carrying a downloadable URL.
func messageWithURL() th.Predicate {
	return func(_ context.Context, update telego.Update) bool {
		m := update.Message
		if m == nil || m.Text == "" || telegram.MessageCommand(m) != "" {
			return false
		}
		return downloader.ExtractFirstURL(m.Text) != ""
	}
}

// messageWithImplicitClip matches non-command messages that parse as implicit clip requests.
func messageWithImplicitClip() th.Predicate {
	return func(_ context.Context, update telego.Update) bool {
		m := update.Message
		if m == nil || m.Text == "" || telegram.MessageCommand(m) != "" {
			return false
		}
		return downloader.DetectImplicitClips(m.Text) != nil
	}
}

// isClipHelpTopic reports whether "/help <topic>" targets a clip command.
func (r *Router) isClipHelpTopic(message *telego.Message) bool {
	parts := strings.Fields(message.Text)
	if len(parts) < 2 {
		return false
	}
	switch strings.TrimPrefix(parts[1], "/") {
	case "clip", "gif", "subs", "mp3":
		return true
	default:
		return false
	}
}
