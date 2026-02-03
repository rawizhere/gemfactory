// Package keyboard defines interfaces for bot keyboard management.
package keyboard

import (
	"context"

	"github.com/mymmrac/telego"
)

// ManagerInterface defines the contract for keyboard interaction.
type ManagerInterface interface {
	GetMainKeyboard() *telego.InlineKeyboardMarkup
	GetAllMonthsKeyboard() *telego.InlineKeyboardMarkup
	HandleCallbackQuery(ctx context.Context, callback *telego.CallbackQuery) error
	Stop()
}
