// Package keyboard defines interfaces for bot keyboard management.
package keyboard

import (
	"context"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ManagerInterface defines the contract for keyboard interaction.
type ManagerInterface interface {
	GetMainKeyboard() tgbotapi.InlineKeyboardMarkup
	GetAllMonthsKeyboard() tgbotapi.InlineKeyboardMarkup
	HandleCallbackQuery(ctx context.Context, callback *tgbotapi.CallbackQuery) error
	Stop()
}
