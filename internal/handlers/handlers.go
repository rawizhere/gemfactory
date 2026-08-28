package handlers

import (
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
	Grok  *GrokHandlers
}

func New(services *service.Services, config *config.Config, keyboard *keyboard.Manager, logger *zap.Logger, tg *telegram.Client, downloads *downloader.Service) *Handlers {
	base := NewBaseHandler(services, config, keyboard, logger, tg)

	user := NewUserHandlers(base)
	admin := NewAdminHandlers(base)
	clip := NewClipHandlers(base, downloads, user)
	grok := NewGrokHandlers(base)

	return &Handlers{
		User:  user,
		Admin: admin,
		Clip:  clip,
		Grok:  grok,
	}
}

func (h *Handlers) RegisterBotCommands() []telego.BotCommand {
	return []telego.BotCommand{
		{Command: "start", Description: "Start the bot"},
		{Command: "help", Description: "Show help and command usage"},
		{Command: "month", Description: "View releases for a month"},
		{Command: "artists", Description: "List tracked artists"},
		{Command: "metrics", Description: "Show system metrics"},
	}
}
