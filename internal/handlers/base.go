package handlers

import (
	"context"
	"gemfactory/internal/config"
	"gemfactory/internal/keyboard"
	"gemfactory/internal/service"
	"gemfactory/internal/telegram"
	"strings"

	"github.com/mymmrac/telego"
	"go.uber.org/zap"
)

type BaseHandler struct {
	Services *service.Services
	Config   *config.Config
	Logger   *zap.Logger
	Keyboard *keyboard.Manager
	TG       *telegram.Client
}

func NewBaseHandler(services *service.Services, config *config.Config, keyboard *keyboard.Manager, logger *zap.Logger, tg *telegram.Client) *BaseHandler {
	return &BaseHandler{
		Services: services,
		Config:   config,
		Logger:   logger,
		Keyboard: keyboard,
		TG:       tg,
	}
}

func (h *BaseHandler) IsAdmin(user *telego.User) bool {
	if user == nil || user.Username == "" {
		return false
	}
	adminUser := strings.TrimPrefix(h.Config.AdminUsername, "@")
	return strings.EqualFold(user.Username, adminUser)
}

func (h *BaseHandler) HandleError(ctx context.Context, chatID int64, err error, userMessage string) {
	h.Logger.Error(userMessage, zap.Error(err), zap.Int64("chat_id", chatID))
	_ = h.TG.SendMessage(ctx, chatID, "Error: "+userMessage)
}
