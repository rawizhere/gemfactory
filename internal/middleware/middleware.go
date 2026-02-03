package middleware

import (
	"gemfactory/internal/config"
	"time"

	"github.com/mymmrac/telego"
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
func (m *Middleware) Process(update telego.Update) bool {
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
func (m *Middleware) ProcessWithMiddleware(update telego.Update, handler func(telego.Update)) {
	middlewareChain := func(update telego.Update) {
		// Recovery middleware
		RecoveryMiddlewareWithUpdate(m.logger)(update, func(update telego.Update) {
			// Logging middleware
			LoggingMiddleware(m.logger)(update, func(update telego.Update) {
				// Debounce middleware for messages
				DebounceMiddleware(m.debouncer, m.logger)(update, func(update telego.Update) {
					// Debounce middleware for callbacks
					DebounceCallbackMiddleware(m.debouncer, m.logger)(update, func(update telego.Update) {
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
func (m *Middleware) ProcessWithMiddlewareAndError(update telego.Update, handler func(telego.Update) error) error {
	middlewareChain := func(update telego.Update) error {
		return ErrorHandlerMiddleware(m.logger)(update, func(update telego.Update) error {
			return LogRequestWithError(m.logger)(update, func(update telego.Update) error {
				return DebounceMiddlewareWithError(m.debouncer, m.logger)(update, func(update telego.Update) error {
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
func (m *Middleware) GetAdminMiddleware() func(update telego.Update, next func(telego.Update)) {
	return AdminOnlyMiddlewareWithConfig(m.config, m.logger)
}

// GetAdminMiddlewareWithError: An error-aware version of GetAdminMiddleware.
func (m *Middleware) GetAdminMiddlewareWithError() func(update telego.Update, next func(telego.Update) error) error {
	return AdminOnlyMiddlewareWithConfigAndError(m.config, m.logger)
}

// Cleanup: Removes expired state from internal middleware components.
func (m *Middleware) Cleanup() {
	m.rateLimiter.Cleanup()
	m.debouncer.Cleanup()
}
