package middleware

import (
	"gemfactory/internal/telegram"
	"runtime/debug"

	"github.com/mymmrac/telego"
	"go.uber.org/zap"
)

func Recovery(logger *zap.Logger) func(update telego.Update, next func(telego.Update)) {
	return func(update telego.Update, next func(telego.Update)) {
		defer func() {
			if panicErr := recover(); panicErr != nil {
				command := ""
				var chatID int64
				var user string

				if update.Message != nil {
					chatID = update.Message.Chat.ID
					user = telegram.GetUserIdentifier(update.Message.From)
					for _, entity := range update.Message.Entities {
						if entity.Type == telego.EntityTypeBotCommand && entity.Offset == 0 {
							command = update.Message.Text[1:entity.Length]
							break
						}
					}
				}

				logger.Error("Panic recovered in update handler",
					zap.String("command", command),
					zap.Int64("chat_id", chatID),
					zap.String("user", user),
					zap.Int("update_id", update.UpdateID),
					zap.Any("panic", panicErr),
					zap.String("stack", string(debug.Stack())))
			}
		}()
		next(update)
	}
}
