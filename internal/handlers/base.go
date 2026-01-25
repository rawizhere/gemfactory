package handlers

import (
	"gemfactory/internal/config"
	"gemfactory/internal/keyboard"
	"gemfactory/internal/service"
	"gemfactory/internal/telegram"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"
)

// BaseHandler provides common dependencies and utility methods for all command handlers.
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

// isAdmin verifies if the given Telegram user has administrative privileges.
func (h *BaseHandler) isAdmin(user *tgbotapi.User) bool {
	if user == nil || user.UserName == "" {
		return false
	}

	adminUser := strings.TrimPrefix(h.Config.AdminUsername, "@")
	match := strings.EqualFold(user.UserName, adminUser)

	if !match {
		h.Logger.Warn("Admin access denied",
			zap.String("user", user.UserName),
			zap.String("expected", adminUser))
	}

	return match
}

// sendMessage transmits a plain text message to the specified chat.
func (h *BaseHandler) sendMessage(chatID int64, text string) {
	if h.BotAPI != nil {
		if err := h.BotAPI.SendMessage(chatID, text); err != nil {
			h.Logger.Error("Failed to send message", zap.Int64("chat_id", chatID), zap.Error(err))
		}
	}
}

// sendMessageWithMarkup transmits a text message with an associated inline keyboard or markup.
func (h *BaseHandler) sendMessageWithMarkup(chatID int64, text string, markup interface{}) {
	if h.BotAPI != nil {
		if err := h.BotAPI.SendMessageWithMarkup(chatID, text, markup); err != nil {
			h.Logger.Error("Failed to send message with markup", zap.Int64("chat_id", chatID), zap.Error(err))
		}
	}
}

// handleError logs an error and notifies the user with a standardized message.
func (h *BaseHandler) handleError(chatID int64, err error, userMessage string) {
	h.Logger.Error(userMessage, zap.Error(err), zap.Int64("chat_id", chatID))
	h.sendMessage(chatID, "❌ "+userMessage)
}

// getMainKeyboard retrieves the primary navigation keyboard for the bot.
func (h *BaseHandler) getMainKeyboard() tgbotapi.InlineKeyboardMarkup {
	return h.Keyboard.GetMainKeyboard()
}
