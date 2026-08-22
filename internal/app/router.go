// Package app handles command and callback routing for incoming Telegram updates.
package app

import (
	"context"
	"strings"

	"gemfactory/internal/config"
	"gemfactory/internal/downloader"
	"gemfactory/internal/handlers"
	"gemfactory/internal/keyboard"
	"gemfactory/internal/middleware"
	"gemfactory/internal/service"
	"gemfactory/internal/telegram"

	"github.com/mymmrac/telego"
	"go.uber.org/zap"
)

// Router dispatches updates to their respective handlers through the middleware pipeline.
type Router struct {
	handlers   *handlers.Handlers
	middleware *middleware.Middleware
	config     *config.Config
	services   *service.Services
	logger     *zap.Logger
}

// NewRouter initializes a Router instance.
func NewRouter(services *service.Services, config *config.Config, keyboard *keyboard.Manager, logger *zap.Logger, tg *telegram.Client, downloads *downloader.Service) *Router {
	return &Router{
		handlers:   handlers.New(services, config, keyboard, logger, tg, downloads),
		middleware: middleware.New(config, logger),
		config:     config,
		services:   services,
		logger:     logger,
	}
}

// HandleUpdate processes a Telegram update with context through the middleware stack.
func (r *Router) HandleUpdate(ctx context.Context, update telego.Update) {
	r.middleware.ProcessWithMiddleware(update, func(update telego.Update) {
		if update.Message != nil {
			r.handleMessage(ctx, update.Message)
		}
		if update.CallbackQuery != nil {
			r.handleCallbackQuery(ctx, update.CallbackQuery)
		}
	})
}

func (r *Router) handleMessage(ctx context.Context, message *telego.Message) {
	command := ""
	for _, entity := range message.Entities {
		if entity.Type == telego.EntityTypeBotCommand && entity.Offset == 0 {
			rawCmd := message.Text[1:entity.Length]
			command = strings.Split(rawCmd, "@")[0]
			break
		}
	}

	if command == "" {
		if u := downloader.ExtractFirstURL(message.Text); u != "" {
			r.handlers.Clip.DirectLink(ctx, message, u)
		}
		return
	}

	switch command {
	case "start":
		r.handlers.User.Start(ctx, message)
	case "help":
		if r.isClipHelpTopic(message) {
			r.handlers.Clip.Help(ctx, message)
		} else {
			r.handlers.User.Help(ctx, message)
		}
	case "clip", "gif", "subs", "mp3":
		switch command {
		case "clip":
			r.handlers.Clip.Clip(ctx, message)
		case "gif":
			r.handlers.Clip.Gif(ctx, message)
		case "subs":
			r.handlers.Clip.Subs(ctx, message)
		case "mp3":
			r.handlers.Clip.MP3(ctx, message)
		}
	case "month":
		r.handlers.User.Month(ctx, message)
	case "search":
		r.handlers.User.Search(ctx, message)
	case "artists":
		r.handlers.User.Artists(ctx, message)
	case "metrics":
		r.handlers.User.Metrics(ctx, message)

	case "admin":
		r.handlers.Admin.Admin(ctx, message)
	case "add_artist":
		r.handlers.Admin.AddArtist(ctx, message)
	case "remove_artist":
		r.handlers.Admin.RemoveArtist(ctx, message)
	case "export":
		r.handlers.Admin.Export(ctx, message)
	case "config":
		r.handlers.Admin.Config(ctx, message)
	case "parse":
		r.handlers.Admin.Parse(ctx, message)
	}
}

func (r *Router) handleCallbackQuery(ctx context.Context, query *telego.CallbackQuery) {
	r.handlers.HandleCallbackQuery(ctx, query)
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

// RegisterBotCommands returns the list of public bot commands for Telegram UI.
func (r *Router) RegisterBotCommands() []telego.BotCommand {
	return r.handlers.RegisterBotCommands()
}
