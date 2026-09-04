// Package app handles command and callback routing for incoming Telegram updates.
package app

import (
	"context"
	"strings"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	"go.uber.org/zap"

	"gemfactory/internal/config"
	"gemfactory/internal/downloader"
	"gemfactory/internal/handlers"
	"gemfactory/internal/keyboard"
	"gemfactory/internal/middleware"
	"gemfactory/internal/service"
	"gemfactory/internal/telegram"
)

// Router registers handlers dispatching updates to their respective
// handler methods through the middleware pipeline.
type Router struct {
	handlers   *handlers.Handlers
	middleware *middleware.Middleware
	config     *config.Config
	services   *service.Services
	logger     *zap.Logger
}

func NewRouter(services *service.Services, config *config.Config, keyboard *keyboard.Manager, logger *zap.Logger, tg *telegram.Client, downloads *downloader.Service) *Router {
	return &Router{
		handlers:   handlers.New(services, config, keyboard, logger, tg, downloads),
		middleware: middleware.New(config, logger),
		config:     config,
		services:   services,
		logger:     logger,
	}
}

// RegisterRoutes wires all command and callback handlers onto the bot handler.
// Handlers match in registration order; only the first matching one runs.
func (r *Router) RegisterRoutes(bh *th.BotHandler) {
	bh.Use(r.middleware.Handlers()...)

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
		{"config", func(ctx *th.Context, m telego.Message) error { r.handlers.Admin.Config(ctx, &m); return nil }},
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
		if mode, ok := detectGrokMode(&m); ok {
			r.handlers.Grok.Run(ctx, &m, mode)
		}
		return nil
	}, grokPredicate())

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
		_, ok := detectGrokMode(update.Message)
		return ok
	}
}

// detectGrokMode matches an @grok reply against the known commands; factcheck is tested first.
func detectGrokMode(m *telego.Message) (handlers.GrokMode, bool) {
	if m == nil || m.ReplyToMessage == nil || m.Text == "" {
		return handlers.GrokMode{}, false
	}
	replacer := strings.NewReplacer(",", " ", ":", " ", "?", " ", "!", " ", ";", " ")
	words := strings.Fields(replacer.Replace(strings.ToLower(m.Text)))
	if len(words) < 2 || words[0] != "@grok" {
		return handlers.GrokMode{}, false
	}

	if len(words) >= 3 {
		if words[1] == "это" && words[2] == "правда" {
			return handlers.GrokFactCheck, true
		}
		if words[1] == "is" && words[2] == "this" && len(words) >= 4 && words[3] == "true" {
			return handlers.GrokFactCheck, true
		}
	}

	switch {
	case strings.HasPrefix(words[1], "переска"),
		words[1] == "retell", words[1] == "summarize", words[1] == "summary", words[1] == "tldr":
		return handlers.GrokRetell, true
	case strings.HasPrefix(words[1], "мнен"),
		words[1] == "opinion", words[1] == "thoughts", words[1] == "think", words[1] == "take":
		return handlers.GrokOpinion, true
	}

	return handlers.GrokMode{}, false
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

func (r *Router) RegisterBotCommands() []telego.BotCommand {
	return r.handlers.RegisterBotCommands()
}
