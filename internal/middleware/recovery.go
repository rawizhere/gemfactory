package middleware

import (
	"errors"
	"gemfactory/internal/model"
	"gemfactory/internal/telegram"
	"runtime/debug"
	"strings"

	"github.com/mymmrac/telego"
	"go.uber.org/zap"
)

// RecoveryMiddlewareWithUpdate handles panics during update processing.
func RecoveryMiddlewareWithUpdate(logger *zap.Logger) func(update telego.Update, next func(telego.Update)) {
	return func(update telego.Update, next func(telego.Update)) {
		defer func() {
			if panicErr := recover(); panicErr != nil {
				if update.Message != nil {
					command := ""
					for _, entity := range update.Message.Entities {
						if entity.Type == telego.EntityTypeBotCommand && entity.Offset == 0 {
							command = update.Message.Text[1:entity.Length]
							break
						}
					}

					user := telegram.GetUserIdentifier(update.Message.From)
					logger.Error("Panic recovered in recovery middleware",
						zap.String("command", command),
						zap.Int64("chat_id", update.Message.Chat.ID),
						zap.String("user", user),
						zap.Int("update_id", update.UpdateID),
						zap.Any("panic", panicErr),
						zap.String("stack", string(debug.Stack())))

				} else {
					logger.Error("Panic recovered in recovery middleware",
						zap.Int("update_id", update.UpdateID),
						zap.Any("panic", panicErr),
						zap.String("stack", string(debug.Stack())))
				}
			}
		}()
		next(update)
	}
}

// ErrorHandlerMiddleware handles errors and panics for handlers.
func ErrorHandlerMiddleware(logger *zap.Logger) func(update telego.Update, next func(telego.Update) error) error {
	return func(update telego.Update, next func(telego.Update) error) error {
		defer func() {
			if panicErr := recover(); panicErr != nil {
				if update.Message != nil {
					command := ""
					for _, entity := range update.Message.Entities {
						if entity.Type == telego.EntityTypeBotCommand && entity.Offset == 0 {
							command = update.Message.Text[1:entity.Length]
							break
						}
					}

					user := telegram.GetUserIdentifier(update.Message.From)
					logger.Error("Panic recovered in error handler",
						zap.String("command", command),
						zap.Int64("chat_id", update.Message.Chat.ID),
						zap.String("user", user),
						zap.Int("update_id", update.UpdateID),
						zap.Any("panic", panicErr),
						zap.String("stack", string(debug.Stack())))

				} else {
					logger.Error("Panic recovered in error handler",
						zap.Int("update_id", update.UpdateID),
						zap.Any("panic", panicErr),
						zap.String("stack", string(debug.Stack())))
				}
			}
		}()

		err := next(update)
		if err != nil && update.Message != nil {
			command := ""
			for _, entity := range update.Message.Entities {
				if entity.Type == telego.EntityTypeBotCommand && entity.Offset == 0 {
					command = update.Message.Text[1:entity.Length]
					break
				}
			}

			user := telegram.GetUserIdentifier(update.Message.From)

			// Categorize error for logging.
			switch {
			case errors.Is(err, model.ErrForbidden), errors.Is(err, model.ErrUnauthorized):
				logger.Warn("Security error",
					zap.String("command", command),
					zap.Int64("chat_id", update.Message.Chat.ID),
					zap.String("user", user),
					zap.Int("update_id", update.UpdateID),
					zap.Error(err))

			case errors.Is(err, model.ErrInvalidInput):
				logger.Warn("Command usage error",
					zap.String("command", command),
					zap.Int64("chat_id", update.Message.Chat.ID),
					zap.String("user", user),
					zap.Int("update_id", update.UpdateID),
					zap.Error(err))

			case errors.Is(err, model.ErrInternal), errors.Is(err, model.ErrRateLimit):
				logger.Error("System error",
					zap.String("command", command),
					zap.Int64("chat_id", update.Message.Chat.ID),
					zap.String("user", user),
					zap.Int("update_id", update.UpdateID),
					zap.Error(err))

			default:
				if isBotError(err) {
					logger.Error("Bot internal error",
						zap.String("command", command),
						zap.Int64("chat_id", update.Message.Chat.ID),
						zap.String("user", user),
						zap.Int("update_id", update.UpdateID),
						zap.Error(err))
				} else {
					logger.Warn("General error",
						zap.String("command", command),
						zap.Int64("chat_id", update.Message.Chat.ID),
						zap.String("user", user),
						zap.Int("update_id", update.UpdateID),
						zap.Error(err))
				}
			}

		}

		return err
	}
}

// isBotError checks if the error is an internal bot error.
func isBotError(err error) bool {
	if err == nil {
		return false
	}
	errorText := strings.ToLower(err.Error())
	return strings.Contains(errorText, "internal") ||
		strings.Contains(errorText, "database") ||
		strings.Contains(errorText, "service")
}
