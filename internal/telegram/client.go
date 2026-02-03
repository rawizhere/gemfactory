package telegram

import (
	"context"
	"fmt"
	"sync"

	"github.com/mymmrac/telego"
	"go.uber.org/zap"
)

// RouterInterface defines methods for routing update events.
type RouterInterface interface {
	HandleUpdate(update telego.Update)
	RegisterBotCommands() []telego.BotCommand
}

// Client maintains the state for a Telegram bot.
type Client struct {
	bot    *telego.Bot
	botAPI BotAPI
	router RouterInterface
	logger *zap.Logger
	config ConfigInterface
	wg     sync.WaitGroup
}

// NewClient initializes a new bot client.
func NewClient(botToken string, config ConfigInterface, logger *zap.Logger) (*Client, error) {
	bot, err := telego.NewBot(botToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot API: %w", err)
	}

	// Create BotAPI wrapper.
	botAPI := NewTelegramBotAPI(bot, logger)

	return &Client{
		bot:    bot,
		botAPI: botAPI,
		logger: logger,
		config: config,
	}, nil
}

// Start begins long-polling for updates.
func (c *Client) Start(ctx context.Context, router RouterInterface) error {
	c.router = router

	// Get bot info.
	me, err := c.bot.GetMe(ctx)
	if err != nil {
		return fmt.Errorf("failed to get bot info: %w", err)
	}
	c.logger.Info("Bot started", zap.String("username", me.Username))

	// Remove webhook if exists.
	err = c.bot.DeleteWebhook(ctx, &telego.DeleteWebhookParams{DropPendingUpdates: true})
	if err != nil {
		c.logger.Error("Failed to delete webhook", zap.Error(err))
		return fmt.Errorf("failed to delete webhook: %w", err)
	}

	// Configure bot commands.
	commands := c.router.RegisterBotCommands()
	err = c.botAPI.SetBotCommands(ctx, commands)
	if err != nil {
		c.logger.Error("Failed to set bot commands", zap.Error(err))
		return fmt.Errorf("failed to set bot commands: %w", err)
	}

	// Configure long polling.
	c.logger.Info("Starting to fetch updates")
	updatesChan, err := c.bot.UpdatesViaLongPolling(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to create updates channel: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("Update loop cancelled by context")
			return ctx.Err()
		case update, ok := <-updatesChan:
			if !ok {
				c.logger.Warn("Update channel closed")
				return fmt.Errorf("update channel closed")
			}

			// Process update concurrently.
			c.wg.Add(1)
			go func(upd telego.Update) {
				defer c.wg.Done()
				c.processUpdate(upd)
			}(update)
		}
	}
}

// processUpdate handles a single Telegram update.
func (c *Client) processUpdate(update telego.Update) {
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
			zap.String("user", GetUserIdentifier(update.Message.From)))
	} else if update.CallbackQuery != nil && update.CallbackQuery.Message != nil {
		c.logger.Info("Received callback",
			zap.String("data", update.CallbackQuery.Data),
			zap.Int64("chat_id", update.CallbackQuery.Message.GetChat().ID),
			zap.String("user", GetUserIdentifier(&update.CallbackQuery.From)))
	}

	if update.Message == nil && update.CallbackQuery == nil {
		return
	}

	// Skip documents and non-commands
	if update.Message != nil {
		if update.Message.Document != nil {
			return
		}
		isCommand := false
		for _, entity := range update.Message.Entities {
			if entity.Type == telego.EntityTypeBotCommand && entity.Offset == 0 {
				isCommand = true
				break
			}
		}
		if !isCommand {
			return
		}
	}

	c.router.HandleUpdate(update)
}

func (c *Client) GetBotAPI() BotAPI {
	return c.botAPI
}

func getUserID(update telego.Update) int64 {
	if update.Message != nil && update.Message.From != nil {
		return update.Message.From.ID
	}
	if update.CallbackQuery != nil {
		return update.CallbackQuery.From.ID
	}
	return 0
}

func extractCommand(update telego.Update) string {
	if update.Message != nil {
		for _, entity := range update.Message.Entities {
			if entity.Type == telego.EntityTypeBotCommand && entity.Offset == 0 {
				return update.Message.Text[1:entity.Length]
			}
		}
	}
	if update.CallbackQuery != nil {
		return "callback"
	}
	return ""
}

func getUpdateType(update telego.Update) string {
	if update.Message != nil {
		for _, entity := range update.Message.Entities {
			if entity.Type == telego.EntityTypeBotCommand && entity.Offset == 0 {
				return "command"
			}
		}
		return "message"
	}
	if update.CallbackQuery != nil {
		return "callback"
	}
	return "unknown"
}
