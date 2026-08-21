package middleware

import (
	"fmt"
	"gemfactory/internal/telegram"
	"time"

	"github.com/mymmrac/telego"
	"go.uber.org/zap"
)

// Logging logs incoming bot commands and their execution duration.
func Logging(logger *zap.Logger) func(update telego.Update, next func(telego.Update)) {
	return func(update telego.Update, next func(telego.Update)) {
		if update.Message == nil {
			next(update)
			return
		}

		command := ""
		for _, entity := range update.Message.Entities {
			if entity.Type == telego.EntityTypeBotCommand && entity.Offset == 0 {
				command = update.Message.Text[1:entity.Length]
				break
			}
		}

		startTime := time.Now()
		reqID := fmt.Sprintf("%d-%d", update.UpdateID, startTime.UnixNano())
		user := telegram.GetUserIdentifier(update.Message.From)

		logger.Info("Processing command",
			zap.String("request_id", reqID),
			zap.String("command", command),
			zap.Int64("user_id", update.Message.From.ID),
			zap.Int64("chat_id", update.Message.Chat.ID),
			zap.String("user", user),
			zap.Int("update_id", update.UpdateID))

		next(update)

		logger.Info("Command completed",
			zap.String("request_id", reqID),
			zap.String("command", command),
			zap.Duration("duration", time.Since(startTime)))
	}
}
