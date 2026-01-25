package scraper

import (
	"context"
	"errors"
	"fmt"
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

		if attempt == config.MaxRetries {
			break
		}

		// Check if error is permanent.
		var permErr *PermanentError
		if errors.As(err, &permErr) {
			return permErr.Err
		}

		logger.Warn("Retry attempt failed",
			zap.Int("attempt", attempt+1),
			zap.Int("max_retries", config.MaxRetries),
			zap.Duration("next_delay", delay),
			zap.Error(err))

		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}

		delay = time.Duration(float64(delay) * config.BackoffMultiplier)
		if delay > config.MaxDelay {
			delay = config.MaxDelay
		}
	}

	return fmt.Errorf("function failed after %d attempts: %w", config.MaxRetries+1, lastErr)
}
