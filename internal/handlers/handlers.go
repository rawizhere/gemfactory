package handlers

import (
	"context"
	"gemfactory/internal/config"
	"gemfactory/internal/downloader"
	"gemfactory/internal/keyboard"
	"gemfactory/internal/service"
	"gemfactory/internal/telegram"

	"github.com/mymmrac/telego"
	"go.uber.org/zap"
)

// Handlers aggregates user, admin, and clip command handlers.
type Handlers struct {
	User  *UserHandlers
	Admin *AdminHandlers
	Clip  *ClipHandlers
}

// New initializes all handler collections.
func New(services *service.Services, config *config.Config, keyboard *keyboard.Manager, logger *zap.Logger, tg *telegram.Client, downloads *downloader.Service) *Handlers {
	base := NewBaseHandler(services, config, keyboard, logger, tg)

	user := NewUserHandlers(base)
	admin := NewAdminHandlers(base)
	clip := NewClipHandlers(base, downloads, user)

	return &Handlers{
		User:  user,
		Admin: admin,
		Clip:  clip,
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
		{Command: "clip", Description: "Вырезать клип из видео"},
		{Command: "gif", Description: "Вырезать клип без звука"},
		{Command: "subs", Description: "Клип с вшитыми субтитрами"},
		{Command: "mp3", Description: "Извлечь аудиодорожку из видео"},
		{Command: "month", Description: "Get releases for a month"},
		{Command: "search", Description: "Search releases by artist"},
		{Command: "artists", Description: "Show artist lists"},
		{Command: "metrics", Description: "Show system metrics"},
	}
}
