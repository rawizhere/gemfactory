package middleware

import (
	"time"

	th "github.com/mymmrac/telego/telegohandler"
	"go.uber.org/zap"
)

// Handlers returns the telegohandler middleware chain, applied in registration order:
// panic recovery, timeout, logging, debouncing, rate limiting. Expired state evicts itself.
func Handlers(logger *zap.Logger) []th.Handler {
	return []th.Handler{
		th.PanicRecovery(),
		th.Timeout(60 * time.Second),
		Logging(logger),
		Debounce(NewDebouncer(1*time.Second, logger), logger),
		RateLimit(NewLimiter(), 10, time.Minute),
	}
}
