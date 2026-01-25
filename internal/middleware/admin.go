// Package middleware manages request processing flows such as rate limiting and debouncing.
package middleware

import (
	"gemfactory/internal/config"
	"gemfactory/internal/telegram"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"
)

// AdminOnlyMiddleware restricts access to users with administrative privileges.
func AdminOnlyMiddleware(adminUsername string, logger *zap.Logger) func(update tgbotapi.Update, next func(tgbotapi.Update)) {
	return func(update tgbotapi.Update, next func(tgbotapi.Update)) {
		if update.Message == nil {
			next(update)
			return
		}

		if update.Message.From == nil {
			logger.Warn("No user information in message")
			return
		}

		if update.Message.From.UserName != adminUsername {
			user := telegram.GetUserIdentifier(update.Message.From)
			logger.Warn("Unauthorized access attempt",
				zap.String("command", update.Message.Command()),
				zap.String("user", user),
				zap.String("expected_admin", adminUsername))
			return
		}

		next(update)
	}
}

// AdminOnlyMiddlewareWithError is an error-aware version of AdminOnlyMiddleware.
func AdminOnlyMiddlewareWithError(adminUsername string, logger *zap.Logger) func(update tgbotapi.Update, next func(tgbotapi.Update) error) error {
	return func(update tgbotapi.Update, next func(tgbotapi.Update) error) error {
		if update.Message == nil {
			return next(update)
		}

		if update.Message.From == nil {
			logger.Warn("No user information in message")
			return nil
		}

		if update.Message.From.UserName != adminUsername {
			user := telegram.GetUserIdentifier(update.Message.From)
			logger.Warn("Unauthorized access attempt",
				zap.String("command", update.Message.Command()),
				zap.String("user", user),
				zap.String("expected_admin", adminUsername))
			return nil
		}

		return next(update)
	}
}

// AdminOnlyMiddlewareWithConfig uses the administrative username from the application configuration.
func AdminOnlyMiddlewareWithConfig(config *config.Config, logger *zap.Logger) func(update tgbotapi.Update, next func(tgbotapi.Update)) {
	return AdminOnlyMiddleware(config.AdminUsername, logger)
}

// AdminOnlyMiddlewareWithConfigAndError is an error-aware version of AdminOnlyMiddlewareWithConfig.
func AdminOnlyMiddlewareWithConfigAndError(config *config.Config, logger *zap.Logger) func(update tgbotapi.Update, next func(tgbotapi.Update) error) error {
	return AdminOnlyMiddlewareWithError(config.AdminUsername, logger)
}
