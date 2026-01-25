// Package telegram provides abstractions and implementations for Telegram bot interactions.
package telegram

import (
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"
)

// BotAPI defines the contract for essential Telegram messaging and management operations.
type BotAPI interface {
	SendMessage(chatID int64, text string) error
	SendMessageWithMarkup(chatID int64, text string, markup any) error
	SendMessageWithReply(chatID int64, text string, replyToMessageID int) error
	SendMessageWithReplyAndMarkup(chatID int64, text string, replyToMessageID int, markup any) error
	EditMessageReplyMarkup(chatID int64, messageID int, markup any) error
	SetBotCommands(commands []tgbotapi.BotCommand) error
	GetFile(fileID string) (tgbotapi.File, error)
}

// TelegramBotAPI is a concrete implementation of BotAPI using the go-telegram-bot-api library.
type TelegramBotAPI struct {
	api    *tgbotapi.BotAPI
	logger *zap.Logger
}

// NewTelegramBotAPI initializes a new TelegramBotAPI instance with logging.
func NewTelegramBotAPI(api *tgbotapi.BotAPI, logger *zap.Logger) *TelegramBotAPI {
	return &TelegramBotAPI{
		api:    api,
		logger: logger,
	}
}

// GetAPI returns the internal Telegram Bot API client.
func (t *TelegramBotAPI) GetAPI() *tgbotapi.BotAPI {
	return t.api
}

// SendMessage transmits a plain text message using HTML parse mode.
func (t *TelegramBotAPI) SendMessage(chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML"
	msg.DisableWebPagePreview = true
	_, err := t.api.Send(msg)
	if err != nil {
		t.logger.Error("Failed to send message", zap.Int64("chat_id", chatID), zap.Error(err))
	}
	return err
}

// SendMessageWithMarkup transmits a text message with an associated reply markup.
func (t *TelegramBotAPI) SendMessageWithMarkup(chatID int64, text string, markup any) error {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = markup
	msg.ParseMode = "HTML"
	msg.DisableWebPagePreview = true
	_, err := t.api.Send(msg)
	if err != nil {
		t.logger.Error("Failed to send message with markup", zap.Int64("chat_id", chatID), zap.Error(err))
	}
	return err
}

// SendMessageWithReply transmits a text message in response to a specific message ID.
func (t *TelegramBotAPI) SendMessageWithReply(chatID int64, text string, replyToMessageID int) error {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyToMessageID = replyToMessageID
	msg.ParseMode = "HTML"
	msg.DisableWebPagePreview = true
	_, err := t.api.Send(msg)
	if err != nil {
		t.logger.Error("Failed to send message with reply", zap.Int64("chat_id", chatID), zap.Int("reply_to_message_id", replyToMessageID), zap.Error(err))
	}
	return err
}

// SendMessageWithReplyAndMarkup transmits a reply message including custom markup.
func (t *TelegramBotAPI) SendMessageWithReplyAndMarkup(chatID int64, text string, replyToMessageID int, markup any) error {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyToMessageID = replyToMessageID
	msg.ReplyMarkup = markup
	msg.ParseMode = "HTML"
	msg.DisableWebPagePreview = true
	_, err := t.api.Send(msg)
	if err != nil {
		t.logger.Error("Failed to send message with reply and markup", zap.Int64("chat_id", chatID), zap.Int("reply_to_message_id", replyToMessageID), zap.Error(err))
	}
	return err
}

// EditMessageReplyMarkup updates the inline keyboard of an existing message.
func (t *TelegramBotAPI) EditMessageReplyMarkup(chatID int64, messageID int, markup any) error {
	inlineMarkup, ok := markup.(tgbotapi.InlineKeyboardMarkup)
	if !ok {
		return fmt.Errorf("markup must be of type tgbotapi.InlineKeyboardMarkup")
	}
	edit := tgbotapi.NewEditMessageReplyMarkup(chatID, messageID, inlineMarkup)
	_, err := t.api.Send(edit)
	if err != nil {
		t.logger.Error("Failed to edit message reply markup", zap.Int64("chat_id", chatID), zap.Int("message_id", messageID), zap.Error(err))
	}
	return err
}

// SetBotCommands registers the bot's command list with the Telegram server.
func (t *TelegramBotAPI) SetBotCommands(commands []tgbotapi.BotCommand) error {
	_, err := t.api.Request(tgbotapi.NewSetMyCommands(commands...))
	if err != nil {
		t.logger.Error("Failed to set bot commands", zap.Error(err))
	}
	return err
}

// GetFile retrieves metadata for a specific file stored on Telegram's servers.
func (t *TelegramBotAPI) GetFile(fileID string) (tgbotapi.File, error) {
	file := tgbotapi.FileConfig{FileID: fileID}
	return t.api.GetFile(file)
}

var _ BotAPI = (*TelegramBotAPI)(nil)
