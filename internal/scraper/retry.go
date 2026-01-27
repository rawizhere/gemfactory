package scraper

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
)

// PermanentError indicate that the operation should not be retried.
type PermanentError struct {
	Err error
}

func (e *PermanentError) Error() string {
	return e.Err.Error()
}

func (e *PermanentError) Unwrap() error {
	return e.Err
}

// WithRetry executes a function repeatedly until it succeeds or the maximum retry count is reached.
func WithRetry(ctx context.Context, logger *zap.Logger, config RetryConfig, fn func() error) error {
	var lastErr error
	delay := config.InitialDelay

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		// Detect 429 Too Many Requests
		isRateLimited := strings.Contains(err.Error(), "429")

		if attempt == config.MaxRetries {
			break
		}

		// Check if error is permanent.
		var permErr *PermanentError
		if errors.As(err, &permErr) {
			return permErr.Err
		}

		actualDelay := delay
		if isRateLimited {
			actualDelay = 5 * time.Second
			logger.Warn("Rate limited (429), backing off...", zap.Int("attempt", attempt+1))
		}

		logger.Warn("Retry attempt failed",
			zap.Int("attempt", attempt+1),
			zap.Int("max_retries", config.MaxRetries),
			zap.Duration("next_delay", actualDelay),
			zap.Error(err))

		select {
		case <-time.After(actualDelay):
		case <-ctx.Done():
			return ctx.Err()
		}

		if isRateLimited {
			delay = 10 * time.Second
		} else {
			delay = time.Duration(float64(delay) * config.BackoffMultiplier)
		}
		if delay > config.MaxDelay {
			delay = config.MaxDelay
		}
	}

	return fmt.Errorf("function failed after %d attempts: %w", config.MaxRetries+1, lastErr)
}
