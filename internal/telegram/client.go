// Package telegram provides abstractions and implementations for Telegram bot interactions.
package telegram

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mymmrac/telego"
	"github.com/mymmrac/telego/telegoutil"
	"go.uber.org/zap"
)

const maxMessageLength = 4000

// Router defines methods for routing update events.
type Router interface {
	HandleUpdate(ctx context.Context, update telego.Update)
	RegisterBotCommands() []telego.BotCommand
}

// Client manages Telegram bot communication, update polling, and message sending.
type Client struct {
	bot    *telego.Bot
	router Router
	logger *zap.Logger
	wg     sync.WaitGroup
}

// NewClient initializes a new Telegram bot client.
func NewClient(botToken string, logger *zap.Logger) (*Client, error) {
	bot, err := telego.NewBot(botToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot API: %w", err)
	}

	return &Client{
		bot:    bot,
		logger: logger,
	}, nil
}

// Start begins long-polling for updates and dispatches them to the router.
func (c *Client) Start(ctx context.Context, router Router) error {
	c.router = router

	me, err := c.bot.GetMe(ctx)
	if err != nil {
		return fmt.Errorf("failed to get bot info: %w", err)
	}
	c.logger.Info("Bot started", zap.String("username", me.Username))

	_ = c.bot.DeleteWebhook(ctx, &telego.DeleteWebhookParams{DropPendingUpdates: true})

	commands := c.router.RegisterBotCommands()
	if err := c.SetBotCommands(ctx, commands); err != nil {
		c.logger.Error("Failed to set bot commands", zap.Error(err))
	}

	c.logger.Info("Starting to fetch updates")
	updatesChan, err := c.bot.UpdatesViaLongPolling(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to create updates channel: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("Update loop cancelled by context")
			c.wg.Wait()
			return ctx.Err()
		case update, ok := <-updatesChan:
			if !ok {
				c.logger.Warn("Update channel closed")
				c.wg.Wait()
				return fmt.Errorf("update channel closed")
			}

			c.wg.Add(1)
			go func(upd telego.Update) {
				defer c.wg.Done()
				updateCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
				defer cancel()
				c.processUpdate(updateCtx, upd)
			}(update)
		}
	}
}

func (c *Client) processUpdate(ctx context.Context, update telego.Update) {
	if update.Message == nil && update.CallbackQuery == nil {
		return
	}

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

	c.router.HandleUpdate(ctx, update)
}

// SendMessage sends an HTML formatted text message, safely splitting if exceeding limit.
func (c *Client) SendMessage(ctx context.Context, chatID int64, text string) error {
	chunks := splitMessage(text, maxMessageLength)
	for _, chunk := range chunks {
		params := telegoutil.Message(telegoutil.ID(chatID), chunk).WithParseMode(telego.ModeHTML)
		params.LinkPreviewOptions = &telego.LinkPreviewOptions{IsDisabled: true}
		_, err := c.bot.SendMessage(ctx, params)
		if err != nil {
			c.logger.Error("Failed to send message", zap.Int64("chat_id", chatID), zap.Error(err))
			return err
		}
	}
	return nil
}

// SendMessageWithMarkup sends an HTML message with reply markup attached to the final chunk.
func (c *Client) SendMessageWithMarkup(ctx context.Context, chatID int64, text string, markup telego.ReplyMarkup) error {
	chunks := splitMessage(text, maxMessageLength)
	for i, chunk := range chunks {
		params := telegoutil.Message(telegoutil.ID(chatID), chunk).WithParseMode(telego.ModeHTML)
		params.LinkPreviewOptions = &telego.LinkPreviewOptions{IsDisabled: true}

		// Attach markup to the last chunk
		if i == len(chunks)-1 && markup != nil {
			params.ReplyMarkup = markup
		}

		_, err := c.bot.SendMessage(ctx, params)
		if err != nil {
			c.logger.Error("Failed to send message with markup", zap.Int64("chat_id", chatID), zap.Error(err))
			return err
		}
	}
	return nil
}

// EditMessageReplyMarkup updates an existing message's keyboard.
func (c *Client) EditMessageReplyMarkup(ctx context.Context, chatID int64, messageID int, markup *telego.InlineKeyboardMarkup) error {
	params := &telego.EditMessageReplyMarkupParams{
		ChatID:      telegoutil.ID(chatID),
		MessageID:   messageID,
		ReplyMarkup: markup,
	}

	_, err := c.bot.EditMessageReplyMarkup(ctx, params)
	if err != nil {
		c.logger.Error("Failed to edit message reply markup", zap.Int64("chat_id", chatID), zap.Int("message_id", messageID), zap.Error(err))
	}
	return err
}

// SetBotCommands registers bot commands with Telegram.
func (c *Client) SetBotCommands(ctx context.Context, commands []telego.BotCommand) error {
	err := c.bot.SetMyCommands(ctx, &telego.SetMyCommandsParams{Commands: commands})
	if err != nil {
		c.logger.Error("Failed to set bot commands", zap.Error(err))
	}
	return err
}

func splitMessage(text string, limit int) []string {
	if len(text) <= limit {
		return []string{text}
	}

	var chunks []string
	lines := strings.Split(text, "\n")
	var current strings.Builder

	for _, line := range lines {
		if current.Len()+len(line)+1 > limit && current.Len() > 0 {
			chunks = append(chunks, current.String())
			current.Reset()
		}

		if current.Len() > 0 {
			current.WriteString("\n")
		}
		current.WriteString(line)
	}

	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}

	return chunks
}
