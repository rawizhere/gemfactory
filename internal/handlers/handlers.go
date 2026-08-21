package handlers

import (
	"context"
	"gemfactory/internal/config"
	"gemfactory/internal/keyboard"
	"gemfactory/internal/service"
	"gemfactory/internal/telegram"

	"github.com/mymmrac/telego"
	"go.uber.org/zap"
)

// Handlers aggregates user and admin command handlers.
type Handlers struct {
	User  *UserHandlers
	Admin *AdminHandlers
}

// New initializes all handler collections.
func New(services *service.Services, config *config.Config, keyboard *keyboard.Manager, logger *zap.Logger, tg *telegram.Client) *Handlers {
	base := NewBaseHandler(services, config, keyboard, logger, tg)

	return &Handlers{
		User:  NewUserHandlers(base),
		Admin: NewAdminHandlers(base),
	}
}

// HandleCallbackQuery routes inline keyboard callback queries to the keyboard manager.
func (h *Handlers) HandleCallbackQuery(ctx context.Context, query *telego.CallbackQuery) {
	err := h.User.Keyboard.HandleCallbackQuery(ctx, query)
	if err != nil {
		h.User.Logger.Error("Failed to handle callback query", zap.Error(err), zap.String("data", query.Data))
	}
}

// RegisterBotCommands returns the command menu configuration for the Telegram bot.
func (h *Handlers) RegisterBotCommands() []telego.BotCommand {
	return []telego.BotCommand{
		{Command: "start", Description: "Start the bot"},
		{Command: "help", Description: "Show help"},
		{Command: "month", Description: "Get releases for a month"},
		{Command: "search", Description: "Search releases by artist"},
		{Command: "artists", Description: "Show artist lists"},
		{Command: "metrics", Description: "Show system metrics"},
	}
}
