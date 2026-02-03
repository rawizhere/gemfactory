// Package telegram provides abstractions and implementations for Telegram bot interactions.
package telegram

import (
	"context"
	"fmt"

	"github.com/mymmrac/telego"
	"github.com/mymmrac/telego/telegoutil"
	"go.uber.org/zap"
)

// BotAPI defines the contract for Telegram messaging and management.
type BotAPI interface {
	SendMessage(ctx context.Context, chatID int64, text string) error
	SendMessageWithMarkup(ctx context.Context, chatID int64, text string, markup any) error
	SendMessageWithReply(ctx context.Context, chatID int64, text string, replyToMessageID int) error
	SendMessageWithReplyAndMarkup(ctx context.Context, chatID int64, text string, replyToMessageID int, markup any) error
	EditMessageReplyMarkup(ctx context.Context, chatID int64, messageID int, markup any) error
	SetBotCommands(ctx context.Context, commands []telego.BotCommand) error
	GetFile(ctx context.Context, fileID string) (*telego.File, error)
}

// TelegramBotAPI implements BotAPI using the telego library.
type TelegramBotAPI struct {
	bot    *telego.Bot
	logger *zap.Logger
}

// NewTelegramBotAPI initializes a new TelegramBotAPI instance with logging.
func NewTelegramBotAPI(bot *telego.Bot, logger *zap.Logger) *TelegramBotAPI {
	return &TelegramBotAPI{
		bot:    bot,
		logger: logger,
	}
}

// GetBot returns the internal telego.Bot client.
func (t *TelegramBotAPI) GetBot() *telego.Bot {
	return t.bot
}

// SendMessage sends a plain text message.
func (t *TelegramBotAPI) SendMessage(ctx context.Context, chatID int64, text string) error {
	params := telegoutil.Message(telegoutil.ID(chatID), text).WithParseMode(telego.ModeHTML)
	params.LinkPreviewOptions = &telego.LinkPreviewOptions{IsDisabled: true}
	_, err := t.bot.SendMessage(ctx, params)
	if err != nil {
		t.logger.Error("Failed to send message", zap.Int64("chat_id", chatID), zap.Error(err))
	}
	return err
}

// SendMessageWithMarkup sends a text message with reply markup.
func (t *TelegramBotAPI) SendMessageWithMarkup(ctx context.Context, chatID int64, text string, markup any) error {
	params := telegoutil.Message(telegoutil.ID(chatID), text).WithParseMode(telego.ModeHTML)
	params.LinkPreviewOptions = &telego.LinkPreviewOptions{IsDisabled: true}

	if markup != nil {
		if m, ok := markup.(telego.ReplyMarkup); ok {
			params.ReplyMarkup = m
		} else {
			return fmt.Errorf("markup must be of type telego.ReplyMarkup")
		}
	}

	_, err := t.bot.SendMessage(ctx, params)
	if err != nil {
		t.logger.Error("Failed to send message with markup", zap.Int64("chat_id", chatID), zap.Error(err))
	}
	return err
}

// SendMessageWithReply sends a text message replying to a specific message.
func (t *TelegramBotAPI) SendMessageWithReply(ctx context.Context, chatID int64, text string, replyToMessageID int) error {
	params := telegoutil.Message(telegoutil.ID(chatID), text).WithParseMode(telego.ModeHTML)
	params.LinkPreviewOptions = &telego.LinkPreviewOptions{IsDisabled: true}
	params.ReplyParameters = &telego.ReplyParameters{MessageID: replyToMessageID}

	_, err := t.bot.SendMessage(ctx, params)
	if err != nil {
		t.logger.Error("Failed to send message with reply", zap.Int64("chat_id", chatID), zap.Int("reply_to_message_id", replyToMessageID), zap.Error(err))
	}
	return err
}

// SendMessageWithReplyAndMarkup sends a reply with custom markup.
func (t *TelegramBotAPI) SendMessageWithReplyAndMarkup(ctx context.Context, chatID int64, text string, replyToMessageID int, markup any) error {
	params := telegoutil.Message(telegoutil.ID(chatID), text).WithParseMode(telego.ModeHTML)
	params.LinkPreviewOptions = &telego.LinkPreviewOptions{IsDisabled: true}
	params.ReplyParameters = &telego.ReplyParameters{MessageID: replyToMessageID}

	if markup != nil {
		if m, ok := markup.(telego.ReplyMarkup); ok {
			params.ReplyMarkup = m
		} else {
			return fmt.Errorf("markup must be of type telego.ReplyMarkup")
		}
	}

	_, err := t.bot.SendMessage(ctx, params)
	if err != nil {
		t.logger.Error("Failed to send message with reply and markup", zap.Int64("chat_id", chatID), zap.Int("reply_to_message_id", replyToMessageID), zap.Error(err))
	}
	return err
}

// EditMessageReplyMarkup updates an existing message's keyboard.
func (t *TelegramBotAPI) EditMessageReplyMarkup(ctx context.Context, chatID int64, messageID int, markup any) error {
	params := &telego.EditMessageReplyMarkupParams{
		ChatID:    telegoutil.ID(chatID),
		MessageID: messageID,
	}

	if markup != nil {
		if m, ok := markup.(*telego.InlineKeyboardMarkup); ok {
			params.ReplyMarkup = m
		} else if m, ok := markup.(telego.InlineKeyboardMarkup); ok {
			params.ReplyMarkup = &m
		} else {
			return fmt.Errorf("markup must be of type telego.InlineKeyboardMarkup")
		}
	}

	_, err := t.bot.EditMessageReplyMarkup(ctx, params)
	if err != nil {
		t.logger.Error("Failed to edit message reply markup", zap.Int64("chat_id", chatID), zap.Int("message_id", messageID), zap.Error(err))
	}
	return err
}

// SetBotCommands registers bot commands with Telegram.
func (t *TelegramBotAPI) SetBotCommands(ctx context.Context, commands []telego.BotCommand) error {
	err := t.bot.SetMyCommands(ctx, &telego.SetMyCommandsParams{Commands: commands})
	if err != nil {
		t.logger.Error("Failed to set bot commands", zap.Error(err))
	}
	return err
}

// GetFile retrieves metadata for a specific file stored on Telegram's servers.
func (t *TelegramBotAPI) GetFile(ctx context.Context, fileID string) (*telego.File, error) {
	return t.bot.GetFile(ctx, &telego.GetFileParams{FileID: fileID})
}

var _ BotAPI = (*TelegramBotAPI)(nil)
