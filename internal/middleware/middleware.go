// Package middleware manages request processing flows such as rate limiting and debouncing.
package middleware

import (
	"gemfactory/internal/config"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"
)

// Middleware orchestrates the update processing pipeline.
type Middleware struct {
	rateLimiter RateLimiterInterface
	debouncer   DebouncerInterface
	logger      *zap.Logger
	config      *config.Config
}

func New(config *config.Config, logger *zap.Logger) *Middleware {
	debouncer := NewDebouncer(1*time.Second, logger)
	rateLimiter := NewRateLimiter(10, 60*time.Second, logger)

	return &Middleware{
		rateLimiter: rateLimiter,
		debouncer:   debouncer,
		logger:      logger,
		config:      config,
	}
}

// Process applies rate limits to the incoming update.
func (m *Middleware) Process(update tgbotapi.Update) bool {
	if update.Message != nil {
		userID := update.Message.From.ID
		if !m.rateLimiter.Allow(userID) {
			m.logger.Warn("Rate limit exceeded", zap.Int64("user_id", userID))
			return false
		}
	}

	return true
}

// ProcessWithMiddleware chains multiple middleware steps before executing the final handler.
func (m *Middleware) ProcessWithMiddleware(update tgbotapi.Update, handler func(tgbotapi.Update)) {
	middlewareChain := func(update tgbotapi.Update) {
		// Recovery middleware
		RecoveryMiddlewareWithUpdate(m.logger)(update, func(update tgbotapi.Update) {
			// Logging middleware
			LoggingMiddleware(m.logger)(update, func(update tgbotapi.Update) {
				// Debounce middleware for messages
				DebounceMiddleware(m.debouncer, m.logger)(update, func(update tgbotapi.Update) {
					// Debounce middleware for callbacks
					DebounceCallbackMiddleware(m.debouncer, m.logger)(update, func(update tgbotapi.Update) {
						// Rate limiting
						if m.Process(update) {
							handler(update)
						}
					})
				})
			})
		})
	}

	middlewareChain(update)
}

// ProcessWithMiddlewareAndError: An error-aware version of ProcessWithMiddleware.
func (m *Middleware) ProcessWithMiddlewareAndError(update tgbotapi.Update, handler func(tgbotapi.Update) error) error {
	middlewareChain := func(update tgbotapi.Update) error {
		return ErrorHandlerMiddleware(m.logger)(update, func(update tgbotapi.Update) error {
			return LogRequestWithError(m.logger)(update, func(update tgbotapi.Update) error {
				return DebounceMiddlewareWithError(m.debouncer, m.logger)(update, func(update tgbotapi.Update) error {
					if !m.Process(update) {
						return nil // Rate limit exceeded, skip handler.
					}
					return handler(update)
				})
			})
		})
	}

	return middlewareChain(update)
}

// GetAdminMiddleware: Returns middleware that restricts access to administrators.
func (m *Middleware) GetAdminMiddleware() func(update tgbotapi.Update, next func(tgbotapi.Update)) {
	return AdminOnlyMiddlewareWithConfig(m.config, m.logger)
}

// GetAdminMiddlewareWithError: An error-aware version of GetAdminMiddleware.
func (m *Middleware) GetAdminMiddlewareWithError() func(update tgbotapi.Update, next func(tgbotapi.Update) error) error {
	return AdminOnlyMiddlewareWithConfigAndError(m.config, m.logger)
}

// Cleanup: Removes expired state from internal middleware components.
func (m *Middleware) Cleanup() {
	m.rateLimiter.Cleanup()
	m.debouncer.Cleanup()
}
