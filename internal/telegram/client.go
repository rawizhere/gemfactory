// Package telegram provides the core client for interacting with the Telegram Bot API.
package telegram

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"
)

// RouterInterface defines the methods required for routing update events.
type RouterInterface interface {
	HandleUpdate(update tgbotapi.Update)
	RegisterBotCommands() []tgbotapi.BotCommand
}

// Client maintains the state and connection for a Telegram bot.
type Client struct {
	bot    *tgbotapi.BotAPI
	botAPI BotAPI
	router RouterInterface
	logger *zap.Logger
	config ConfigInterface
	wg     sync.WaitGroup
}

// NewClient initializes a new Telegram bot client instance.
func NewClient(botToken string, config ConfigInterface, logger *zap.Logger) (*Client, error) {
	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot API: %w", err)
	}

	bot.Debug = false
	logger.Info("Telegram bot created", zap.String("username", bot.Self.UserName))

	// Create BotAPI wrapper.
	botAPI := NewTelegramBotAPI(bot, logger)

	return &Client{
		bot:    bot,
		botAPI: botAPI,
		logger: logger,
		config: config,
	}, nil
}

// Start initiates the long-polling process for receiving updates.
func (c *Client) Start(ctx context.Context, router RouterInterface) error {
	c.router = router

	// Initialize bot.
	c.logger.Info("Bot started", zap.String("username", c.bot.Self.UserName))

	// Remove webhook if exists.
	_, err := c.bot.Request(tgbotapi.DeleteWebhookConfig{DropPendingUpdates: true})
	if err != nil {
		c.logger.Error("Failed to delete webhook", zap.Error(err))
		return fmt.Errorf("failed to delete webhook: %w", err)
	}

	// Configure bot commands.
	commands := c.router.RegisterBotCommands()
	_, err = c.bot.Request(tgbotapi.NewSetMyCommands(commands...))
	if err != nil {
		c.logger.Error("Failed to set bot commands", zap.Error(err))
		return fmt.Errorf("failed to set bot commands: %w", err)
	}

	// Configure long polling.
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	u.AllowedUpdates = []string{"message", "callback_query"}

	c.logger.Info("Starting to fetch updates")
	updatesChan := c.bot.GetUpdatesChan(u)
	if updatesChan == nil {
		return fmt.Errorf("failed to create updates channel")
	}

	reconnectDelay := 10 * time.Second
	maxReconnectAttempts := 5
	reconnectAttempts := 0

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("Update loop cancelled by context")
			return ctx.Err()
		case update, ok := <-updatesChan:
			if !ok {
				c.logger.Warn("Update channel closed, attempting to reconnect",
					zap.Int("attempt", reconnectAttempts+1),
					zap.Int("max_attempts", maxReconnectAttempts))

				reconnectAttempts++
				if reconnectAttempts > maxReconnectAttempts {
					c.logger.Error("Max reconnection attempts reached, giving up")
					return fmt.Errorf("max reconnection attempts reached")
				}

				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(reconnectDelay):
					updatesChan = c.bot.GetUpdatesChan(u)
					if updatesChan == nil {
						c.logger.Error("Failed to recreate updates channel")
						continue
					}
					c.logger.Info("Successfully recreated updates channel")
					continue
				}
			}

			reconnectAttempts = 0

			// Process update concurrently.
			c.wg.Add(1)
			go func(upd tgbotapi.Update) {
				defer c.wg.Done()
				c.processUpdate(upd)
			}(update)
		}
	}
}

// processUpdate analyzes and delegates a single Telegram update to the router.
func (c *Client) processUpdate(update tgbotapi.Update) {
	// Enhanced logging with helper functions.
	c.logger.Debug("Processing update",
		zap.Int("update_id", update.UpdateID),
		zap.Int64("user_id", getUserID(update)),
		zap.String("command", extractCommand(update)),
		zap.String("update_type", getUpdateType(update)),
	)

	if update.Message != nil {
		c.logger.Debug("Received message",
			zap.String("text", update.Message.Text),
			zap.Int64("chat_id", update.Message.Chat.ID),
			zap.String("user", getUserIdentifier(update.Message.From)),
			zap.Int("update_id", update.UpdateID))
	} else if update.CallbackQuery != nil {
		month := extractMonth(update.CallbackQuery.Data)
		c.logger.Info("Received callback",
			zap.String("data", update.CallbackQuery.Data),
			zap.String("month", month),
			zap.Int64("chat_id", update.CallbackQuery.Message.Chat.ID),
			zap.String("user", getUserIdentifier(update.CallbackQuery.From)))
		c.logger.Debug("Callback details",
			zap.Int("update_id", update.UpdateID))
	}

	if update.Message == nil && update.CallbackQuery == nil {
		return
	}

	// Skip file attachments (not processed).
	if update.Message != nil && update.Message.Document != nil {
		return
	}

	// Process only commands.
	if update.Message != nil && !update.Message.IsCommand() {
		return
	}

	c.router.HandleUpdate(update)
}

// GetBotAPI returns the abstraction layer for sending messages.
func (c *Client) GetBotAPI() BotAPI {
	return c.botAPI
}

// getUserID extracts the user's ID from an update.
func getUserID(update tgbotapi.Update) int64 {
	if update.Message != nil {
		return update.Message.From.ID
	}
	if update.CallbackQuery != nil {
		return update.CallbackQuery.From.ID
	}
	return 0
}

// extractCommand retrieves the command string from a message or callback update.
func extractCommand(update tgbotapi.Update) string {
	if update.Message != nil && update.Message.IsCommand() {
		return update.Message.Command()
	}
	if update.CallbackQuery != nil {
		return "callback"
	}
	return ""
}

// getUpdateType identifies the category of the incoming Telegram update.
func getUpdateType(update tgbotapi.Update) string {
	if update.Message != nil {
		if update.Message.IsCommand() {
			return "command"
		}
		return "message"
	}
	if update.CallbackQuery != nil {
		return "callback"
	}
	return "unknown"
}

// getUserIdentifier converts a Telegram user object into a human-readable string.
func getUserIdentifier(user *tgbotapi.User) string {
	if user == nil {
		return "unknown"
	}

	if user.UserName != "" {
		return "@" + user.UserName
	}

	if user.FirstName != "" {
		if user.LastName != "" {
			return user.FirstName + " " + user.LastName
		}
		return user.FirstName
	}

	return fmt.Sprintf("user_%d", user.ID)
}

// extractMonth retrieves the month name from button callback metadata.
func extractMonth(data string) string {
	if strings.HasPrefix(data, "month_") {
		return strings.TrimPrefix(data, "month_")
	}
	return "unknown"
}
