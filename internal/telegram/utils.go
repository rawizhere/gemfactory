package telegram

import (
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// GetUserIdentifier determines a human-readable name for a Telegram user.
func GetUserIdentifier(user *tgbotapi.User) string {
	if user == nil {
		return "unknown"
	}
	if user.UserName != "" {
		return user.UserName
	}
	if user.FirstName != "" {
		if user.LastName != "" {
			return user.FirstName + " " + user.LastName
		}
		return user.FirstName
	}
	return fmt.Sprintf("user_%d", user.ID)
}
