package middleware

import (
	"fmt"
	"time"

	"gemfactory/internal/telegram"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	"log/slog"
)

// Logging logs incoming commands and their duration.
func Logging(logger *slog.Logger) th.Handler {
	return func(ctx *th.Context, update telego.Update) error {
		if update.Message == nil || update.Message.From == nil {
			if q := update.CallbackQuery; q != nil {
				logger.Info("Callback query",
					slog.Int64("user_id", q.From.ID),
					slog.String("user", telegram.GetUserIdentifier(&q.From)),
					slog.String("data", q.Data))
			}
			return ctx.Next(update)
		}

		command := telegram.MessageCommand(update.Message)
		startTime := time.Now()
		reqID := fmt.Sprintf("%d-%d", update.UpdateID, startTime.UnixNano())
		user := telegram.GetUserIdentifier(update.Message.From)

		logger.Info("Processing command",
			slog.String("request_id", reqID),
			slog.String("command", command),
			slog.Int64("user_id", update.Message.From.ID),
			slog.Int64("chat_id", update.Message.Chat.ID),
			slog.String("user", user),
			slog.Int("update_id", update.UpdateID))

		err := ctx.Next(update)

		logger.Info("Command completed",
			slog.String("request_id", reqID),
			slog.String("command", command),
			slog.Duration("duration", time.Since(startTime)))

		return err
	}
}
