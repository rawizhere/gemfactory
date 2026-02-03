package app

import (
	"context"
	"gemfactory/internal/config"
	"gemfactory/internal/handlers"
	"gemfactory/internal/middleware"
	"gemfactory/internal/service"
	"gemfactory/internal/telegram"

	"github.com/mymmrac/telego"
	"go.uber.org/zap"
)

// Router directs incoming updates to handlers.
type Router struct {
	handlers   *handlers.Handlers
	middleware *middleware.Middleware
	config     *config.Config
	services   *service.Services
	logger     *zap.Logger
}

func NewRouterWithBotAPI(services *service.Services, config *config.Config, logger *zap.Logger, botAPI telegram.BotAPI) *Router {
	return &Router{
		handlers:   handlers.RegisterRoutesWithBotAPI(services, config, logger, botAPI),
		middleware: middleware.New(config, logger),
		config:     config,
		services:   services,
		logger:     logger,
	}
}

// HandleUpdate processes a Telegram update.
func (r *Router) HandleUpdate(update telego.Update) {
	ctx := context.Background()

	r.middleware.ProcessWithMiddleware(update, func(update telego.Update) {
		if update.Message != nil {
			r.handleMessage(ctx, update.Message)
		}
		if update.CallbackQuery != nil {
			r.handleCallbackQuery(ctx, update.CallbackQuery)
		}
	})
}

// handleMessage handles text commands.
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

func (r *Router) handleCallbackQuery(ctx context.Context, query *telego.CallbackQuery) {
	r.handlers.HandleCallbackQuery(ctx, query)
}

func (r *Router) RegisterBotCommands() []telego.BotCommand {
	return r.handlers.RegisterBotCommands()
}
