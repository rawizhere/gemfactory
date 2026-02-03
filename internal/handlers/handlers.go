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

// Handlers aggregates all domain-specific command handlers into a single container.
type Handlers struct {
	User     *UserHandlers
	Admin    *AdminHandlers
	Homework *HomeworkHandlers
}

// RegisterRoutesWithBotAPI initializes and configures all handlers and their dependencies.
func RegisterRoutesWithBotAPI(services *service.Services, config *config.Config, logger *zap.Logger, botAPI telegram.BotAPI) *Handlers {
	// Initialize keyboard manager
	keyboardManager := keyboard.NewKeyboardManager(services.Release, config, logger)

	// Set BotAPI in keyboard manager
	keyboardManager.SetBotAPI(botAPI)

	// Initialize handlers
	return New(services, config, keyboardManager, logger, botAPI)
}

// New initializes a new handlers container with its required internal services.
func New(services *service.Services, config *config.Config, keyboard keyboard.ManagerInterface, logger *zap.Logger, botAPI telegram.BotAPI) *Handlers {
	base := NewBaseHandler(services, config, keyboard, logger, botAPI)

	return &Handlers{
		User:     NewUserHandlers(base),
		Admin:    NewAdminHandlers(base),
		Homework: NewHomeworkHandlers(base),
	}
}

// HandleCallbackQuery delegates Telegram callback query interaction to the keyboard manager.
func (h *Handlers) HandleCallbackQuery(ctx context.Context, query *telego.CallbackQuery) {
	err := h.User.Keyboard.HandleCallbackQuery(ctx, query)
	if err != nil {
		h.User.Logger.Error("Failed to handle callback query", zap.Error(err), zap.String("data", query.Data))
	}
}

// RegisterBotCommands returns a slice of standard Telegram bot commands for menu registration.
func (h *Handlers) RegisterBotCommands() []telego.BotCommand {
	return []telego.BotCommand{
		{Command: "start", Description: "Start the bot"},
		{Command: "help", Description: "Show help"},
		{Command: "month", Description: "Get releases for a month"},
		{Command: "search", Description: "Search releases by artist"},
		{Command: "artists", Description: "Show artist lists"},
		{Command: "metrics", Description: "Show system metrics"},
		{Command: "homework", Description: "Get a random homework assignment"},
		{Command: "playlist", Description: "Playlist information"},
	}
}
