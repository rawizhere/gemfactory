package telegram

import (
	"fmt"
	"strings"

	"github.com/mymmrac/telego"
)

func GetUserIdentifier(user *telego.User) string {
	if user == nil {
		return "unknown"
	}
	if user.Username != "" {
		return user.Username
	}
	if user.FirstName != "" {
		if user.LastName != "" {
			return user.FirstName + " " + user.LastName
		}
		return user.FirstName
	}
	return fmt.Sprintf("user_%d", user.ID)
}

// MessageCommand extracts the bot command name ("/clip@BotName" -> "clip"), or "" if absent.
func MessageCommand(message *telego.Message) string {
	if message == nil || message.Text == "" {
		return ""
	}
	for _, entity := range message.Entities {
		if entity.Type == telego.EntityTypeBotCommand && entity.Offset == 0 {
			rawCmd := message.Text[1:entity.Length]
			return strings.Split(rawCmd, "@")[0]
		}
	}
	return ""
}
