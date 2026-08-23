package middleware

import (
	"fmt"
	"time"

	"gemfactory/internal/telegram"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	"go.uber.org/zap"
)

// Logging logs incoming commands and their duration.
func Logging(logger *zap.Logger) th.Handler {
	return func(ctx *th.Context, update telego.Update) error {
		if update.Message == nil || update.Message.From == nil {
			return ctx.Next(update)
		}

		command := telegram.MessageCommand(update.Message)
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

		err := ctx.Next(update)

		logger.Info("Command completed",
			zap.String("request_id", reqID),
			zap.String("command", command),
			zap.Duration("duration", time.Since(startTime)))

		return err
	}
}
