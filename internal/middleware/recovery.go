// Package middleware manages request processing flows such as rate limiting and debouncing.
package middleware

import (
	"errors"
	"gemfactory/internal/model"
	"runtime/debug"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"
)

// RecoveryMiddlewareWithUpdate handles panics during update processing.
func RecoveryMiddlewareWithUpdate(logger *zap.Logger) func(update tgbotapi.Update, next func(tgbotapi.Update)) {
	return func(update tgbotapi.Update, next func(tgbotapi.Update)) {
		defer func() {
			if panicErr := recover(); panicErr != nil {
				if update.Message != nil {
					user := getUserIdentifier(update.Message.From)
					logger.Error("Panic recovered in recovery middleware",
						zap.String("command", update.Message.Command()),
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

// ErrorHandlerMiddleware provides unified error handling and panic recovery for handlers.
func ErrorHandlerMiddleware(logger *zap.Logger) func(update tgbotapi.Update, next func(tgbotapi.Update) error) error {
	return func(update tgbotapi.Update, next func(tgbotapi.Update) error) error {
		defer func() {
			if panicErr := recover(); panicErr != nil {
				if update.Message != nil {
					user := getUserIdentifier(update.Message.From)
					logger.Error("Panic recovered in error handler",
						zap.String("command", update.Message.Command()),
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
			user := getUserIdentifier(update.Message.From)

			// Categorize error for logging.
			switch {
			case errors.Is(err, model.ErrForbidden), errors.Is(err, model.ErrUnauthorized):
				logger.Warn("Security error",
					zap.String("command", update.Message.Command()),
					zap.Int64("chat_id", update.Message.Chat.ID),
					zap.String("user", user),
					zap.Int("update_id", update.UpdateID),
					zap.Error(err))

			case errors.Is(err, model.ErrInvalidInput):
				logger.Warn("Command usage error",
					zap.String("command", update.Message.Command()),
					zap.Int64("chat_id", update.Message.Chat.ID),
					zap.String("user", user),
					zap.Int("update_id", update.UpdateID),
					zap.Error(err))

			case errors.Is(err, model.ErrInternal), errors.Is(err, model.ErrRateLimit):
				logger.Error("System error",
					zap.String("command", update.Message.Command()),
					zap.Int64("chat_id", update.Message.Chat.ID),
					zap.String("user", user),
					zap.Int("update_id", update.UpdateID),
					zap.Error(err))

			default:
				if isBotError(err) {
					logger.Error("Bot internal error",
						zap.String("command", update.Message.Command()),
						zap.Int64("chat_id", update.Message.Chat.ID),
						zap.String("user", user),
						zap.Int("update_id", update.UpdateID),
						zap.Error(err))
				} else {
					logger.Warn("General error",
						zap.String("command", update.Message.Command()),
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

// isBotError detects if an error originates from internal bot services.
func isBotError(err error) bool {
	if err == nil {
		return false
	}
	errorText := strings.ToLower(err.Error())
	return strings.Contains(errorText, "internal") ||
		strings.Contains(errorText, "database") ||
		strings.Contains(errorText, "service")
}
