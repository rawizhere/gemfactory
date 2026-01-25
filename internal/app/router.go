// Package app provides the command routing logic for Telegram updates.
package app

import (
	"context"
	"gemfactory/internal/config"
	"gemfactory/internal/handlers"
	"gemfactory/internal/middleware"
	"gemfactory/internal/service"
	"gemfactory/internal/telegram"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"
)

// Router directs incoming Telegram updates to their appropriate command handlers.
type Router struct {
	handlers   *handlers.Handlers
	middleware *middleware.Middleware
	config     *config.Config
	services   *service.Services
	logger     *zap.Logger
}

// NewRouterWithBotAPI initializes a new Router with the specified services and BotAPI.
func NewRouterWithBotAPI(services *service.Services, config *config.Config, logger *zap.Logger, botAPI telegram.BotAPI) *Router {
	return &Router{
		handlers:   handlers.RegisterRoutesWithBotAPI(services, config, logger, botAPI),
		middleware: middleware.New(config, logger),
		config:     config,
		services:   services,
		logger:     logger,
	}
}

// HandleUpdate processes a generic Telegram update through the middleware and handlers.
func (r *Router) HandleUpdate(update tgbotapi.Update) {
	// Create context for the update processing lifecycle
	ctx := context.Background()

	// Apply global middleware chain
	r.middleware.ProcessWithMiddleware(update, func(update tgbotapi.Update) {
		// Handle text messages
		if update.Message != nil {
			r.handleMessage(ctx, update.Message)
		}

		// Handle callback queries
		if update.CallbackQuery != nil {
			r.handleCallbackQuery(ctx, update.CallbackQuery)
		}
	})
}

// handleMessage dispatches text-based commands to their respective user or admin handlers.
func (r *Router) handleMessage(ctx context.Context, message *tgbotapi.Message) {
	if !message.IsCommand() {
		return
	}

	command := strings.ToLower(message.Command())

	switch command {
	// User commands
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

	// Homework commands
	case "homework":
		r.handlers.Homework.Homework(ctx, message)
	case "playlist":
		r.handlers.Homework.Playlist(ctx, message)

	// Admin commands (permission check is done inside the handler)
	case "admin":
		r.handlers.Admin.Admin(ctx, message)
	case "add_artist":
		r.handlers.Admin.AddArtist(ctx, message)
	case "remove_artist":
		r.handlers.Admin.RemoveArtist(ctx, message)
	case "export":
		r.handlers.Admin.Export(ctx, message)
	case "parse":
		r.handlers.Admin.Parse(ctx, message)
	}
}

// handleCallbackQuery processes interaction events from inline keyboards.
func (r *Router) handleCallbackQuery(ctx context.Context, query *tgbotapi.CallbackQuery) {
	r.handlers.HandleCallbackQuery(ctx, query)
}

// RegisterBotCommands retrieves the collection of standard commands for bot menu registration.
func (r *Router) RegisterBotCommands() []tgbotapi.BotCommand {
	return r.handlers.RegisterBotCommands()
}
