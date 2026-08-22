package middleware

import (
	"gemfactory/internal/config"
	"time"

	"github.com/mymmrac/telego"
	"go.uber.org/zap"
)

type Middleware struct {
	rateLimiter *RateLimiter
	debouncer   *Debouncer
	logger      *zap.Logger
	config      *config.Config
}

func New(config *config.Config, logger *zap.Logger) *Middleware {
	return &Middleware{
		rateLimiter: NewRateLimiter(10, 60*time.Second, logger),
		debouncer:   NewDebouncer(1*time.Second, logger),
		logger:      logger,
		config:      config,
	}
}

func (m *Middleware) Process(update telego.Update) bool {
	if update.Message != nil && update.Message.From != nil {
		userID := update.Message.From.ID
		if !m.rateLimiter.Allow(userID) {
			m.logger.Warn("Rate limit exceeded", zap.Int64("user_id", userID))
			return false
		}
	}
	return true
}

func (m *Middleware) ProcessWithMiddleware(update telego.Update, handler func(telego.Update)) {
	Recovery(m.logger)(update, func(update telego.Update) {
		Logging(m.logger)(update, func(update telego.Update) {
			Debounce(m.debouncer, m.logger)(update, func(update telego.Update) {
				DebounceCallback(m.debouncer, m.logger)(update, func(update telego.Update) {
					if m.Process(update) {
						handler(update)
					}
				})
			})
		})
	})
}

func (m *Middleware) Cleanup() {
	m.rateLimiter.Cleanup()
	m.debouncer.Cleanup()
}
