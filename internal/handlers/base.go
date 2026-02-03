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

// BaseHandler provides common dependencies for handlers.
type BaseHandler struct {
	Services *service.Services
	Config   *config.Config
	Logger   *zap.Logger
	Keyboard keyboard.ManagerInterface
	BotAPI   telegram.BotAPI
}

// NewBaseHandler initializes a new BaseHandler instance.
func NewBaseHandler(services *service.Services, config *config.Config, keyboard keyboard.ManagerInterface, logger *zap.Logger, botAPI telegram.BotAPI) *BaseHandler {
	return &BaseHandler{
		Services: services,
		Config:   config,
		Logger:   logger,
		Keyboard: keyboard,
		BotAPI:   botAPI,
	}
}

// IsAdmin checks if the user has admin privileges.
func (h *BaseHandler) IsAdmin(user *telego.User) bool {
	if user == nil || user.Username == "" {
		return false
	}

	adminUser := strings.TrimPrefix(h.Config.AdminUsername, "@")
	match := strings.EqualFold(user.Username, adminUser)

	if !match {
		h.Logger.Warn("Admin access denied",
			zap.String("user", user.Username),
			zap.String("expected", adminUser))
	}

	return match
}

// SendMessage sends a plain text message.
func (h *BaseHandler) SendMessage(ctx context.Context, chatID int64, text string) error {
	return h.BotAPI.SendMessage(ctx, chatID, text)
}

// SendMessageWithMarkup sends a message with reply markup.
func (h *BaseHandler) SendMessageWithMarkup(ctx context.Context, chatID int64, text string, markup any) error {
	return h.BotAPI.SendMessageWithMarkup(ctx, chatID, text, markup)
}

// SendMessageWithKeyboard sends a message with the default navigation keyboard.
func (h *BaseHandler) SendMessageWithKeyboard(ctx context.Context, chatID int64, text string) error {
	return h.BotAPI.SendMessageWithMarkup(ctx, chatID, text, h.Keyboard.GetMainKeyboard())
}

// HandleError logs and notifies the user about an error.
func (h *BaseHandler) HandleError(ctx context.Context, chatID int64, err error, userMessage string) {
	h.Logger.Error(userMessage, zap.Error(err), zap.Int64("chat_id", chatID))
	_ = h.BotAPI.SendMessage(ctx, chatID, "❌ "+userMessage)
}

// GetMainKeyboard retrieves the primary navigation markup from the keyboard manager.
func (h *BaseHandler) GetMainKeyboard() *telego.InlineKeyboardMarkup {
	return h.Keyboard.GetMainKeyboard()
}
