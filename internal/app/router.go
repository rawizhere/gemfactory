// Package app handles command and callback routing for incoming Telegram updates.
package app

import (
	"context"
	"gemfactory/internal/config"
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
func NewRouter(services *service.Services, config *config.Config, keyboard *keyboard.Manager, logger *zap.Logger, tg *telegram.Client) *Router {
	return &Router{
		handlers:   handlers.New(services, config, keyboard, logger, tg),
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
			command = message.Text[1:entity.Length]
			break
		}
	}

	if command == "" {
		return
	}

	switch command {
	case "start":
		r.handlers.User.Start(ctx, message)
	case "help":
		r.handlers.User.Help(ctx, message)
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

// RegisterBotCommands returns the list of public bot commands for Telegram UI.
func (r *Router) RegisterBotCommands() []telego.BotCommand {
	return r.handlers.RegisterBotCommands()
}
