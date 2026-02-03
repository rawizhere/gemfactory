package middleware

import (
	"gemfactory/internal/config"
	"strings"

	"github.com/mymmrac/telego"
	"go.uber.org/zap"
)

// AdminOnlyMiddleware restricts access to administrators.
func AdminOnlyMiddleware(adminUsername string, logger *zap.Logger) func(update telego.Update, next func(telego.Update)) {
	return func(update telego.Update, next func(telego.Update)) {
		if update.Message == nil {
			next(update)
			return
		}

		if update.Message.From == nil {
			logger.Warn("No user information in message")
			return
		}

		cleanAdmin := strings.TrimPrefix(adminUsername, "@")
		if !strings.EqualFold(update.Message.From.Username, cleanAdmin) {
			logger.Warn("Unauthorized access attempt",
				zap.String("text", update.Message.Text),
				zap.String("user", update.Message.From.Username),
				zap.String("expected_admin", cleanAdmin))
			return
		}

		next(update)
	}
}

// AdminOnlyMiddlewareWithError is an error-aware version of AdminOnlyMiddleware.
func AdminOnlyMiddlewareWithError(adminUsername string, logger *zap.Logger) func(update telego.Update, next func(telego.Update) error) error {
	return func(update telego.Update, next func(telego.Update) error) error {
		if update.Message == nil {
			return next(update)
		}

		if update.Message.From == nil {
			logger.Warn("No user information in message")
			return nil
		}

		cleanAdmin := strings.TrimPrefix(adminUsername, "@")
		if !strings.EqualFold(update.Message.From.Username, cleanAdmin) {
			logger.Warn("Unauthorized access attempt",
				zap.String("text", update.Message.Text),
				zap.String("user", update.Message.From.Username),
				zap.String("expected_admin", cleanAdmin))
			return nil
		}

		return next(update)
	}
}

// AdminOnlyMiddlewareWithConfig uses the admin username from config.
func AdminOnlyMiddlewareWithConfig(config *config.Config, logger *zap.Logger) func(update telego.Update, next func(telego.Update)) {
	return AdminOnlyMiddleware(config.AdminUsername, logger)
}

// AdminOnlyMiddlewareWithConfigAndError is an error-aware version of AdminOnlyMiddlewareWithConfig.
func AdminOnlyMiddlewareWithConfigAndError(config *config.Config, logger *zap.Logger) func(update telego.Update, next func(telego.Update) error) error {
	return AdminOnlyMiddlewareWithError(config.AdminUsername, logger)
}
