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

	// Non-command messages containing a URL start a direct download; registered last so commands win.
	bh.HandleMessage(func(ctx *th.Context, m telego.Message) error {
		r.handlers.Clip.DirectLink(ctx, &m, downloader.ExtractFirstURL(m.Text))
		return nil
	}, messageWithURL())

	bh.HandleCallbackQuery(func(ctx *th.Context, q telego.CallbackQuery) error {
		r.handlers.HandleCallbackQuery(ctx, &q)
		return nil
	}, th.AnyCallbackQuery())
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
