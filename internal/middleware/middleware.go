package middleware

import (
	"gemfactory/internal/config"
	"time"

	th "github.com/mymmrac/telego/telegohandler"
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

// Handlers returns the telegohandler middleware chain, applied in registration order:
// panic recovery, timeout, logging, debouncing, rate limiting. Expired state evicts itself.
func (m *Middleware) Handlers() []th.Handler {
	return []th.Handler{
		th.PanicRecovery(),
		th.Timeout(60 * time.Second),
		Logging(m.logger),
		Debounce(m.debouncer, m.logger),
		RateLimit(m.rateLimiter),
	}
}
