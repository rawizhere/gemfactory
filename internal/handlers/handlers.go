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

type Handlers struct {
	User  *UserHandlers
	Admin *AdminHandlers
	Clip  *ClipHandlers
}

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

func (h *Handlers) HandleCallbackQuery(ctx context.Context, query *telego.CallbackQuery) {
	err := h.User.Keyboard.HandleCallbackQuery(ctx, query)
	if err != nil {
		h.User.Logger.Error("Failed to handle callback query", zap.Error(err), zap.String("data", query.Data))
	}
}

func (h *Handlers) RegisterBotCommands() []telego.BotCommand {
	return []telego.BotCommand{
		{Command: "start", Description: "Start the bot"},
		{Command: "help", Description: "Show help and command usage"},
		{Command: "clip", Description: "Cut video clip"},
		{Command: "gif", Description: "Cut video clip without audio"},
		{Command: "subs", Description: "Cut clip with burned-in subtitles"},
		{Command: "mp3", Description: "Extract audio track as MP3"},
		{Command: "month", Description: "View releases for a month"},
		{Command: "search", Description: "Search releases by artist"},
		{Command: "artists", Description: "List tracked artists"},
		{Command: "metrics", Description: "Show system metrics"},
	}
}
