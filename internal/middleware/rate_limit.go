package middleware

import (
	"slices"
	"sync"
	"time"

	"go.uber.org/zap"
)

// RateLimiter enforces sliding-window request limits per user.
type RateLimiter struct {
	requests map[int64][]time.Time
	mu       sync.RWMutex
	limit    int
	window   time.Duration
	logger   *zap.Logger
}

// NewRateLimiter creates a new RateLimiter instance.
func NewRateLimiter(limit int, window time.Duration, logger *zap.Logger) *RateLimiter {
	return &RateLimiter{
		requests: make(map[int64][]time.Time),
		limit:    limit,
		window:   window,
		logger:   logger,
	}
}

// Allow determines if the user has remaining quota in the current window.
func (rl *RateLimiter) Allow(userID int64) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-rl.window)

	requests, exists := rl.requests[userID]
	if !exists {
		rl.requests[userID] = []time.Time{now}
		return true
	}

	requests = slices.DeleteFunc(requests, func(reqTime time.Time) bool {
		return !reqTime.After(windowStart)
	})

	if len(requests) >= rl.limit {
		rl.logger.Warn("Rate limit exceeded",
			zap.Int64("user_id", userID),
			zap.Int("requests", len(requests)),
			zap.Int("limit", rl.limit))
		return false
	}

	requests = append(requests, now)
	rl.requests[userID] = requests

	return true
}

// Cleanup removes expired request records from the tracking map.
func (rl *RateLimiter) Cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-rl.window)

	for userID, requests := range rl.requests {
		validRequests := slices.DeleteFunc(requests, func(reqTime time.Time) bool {
			return !reqTime.After(windowStart)
		})

		if len(validRequests) == 0 {
			delete(rl.requests, userID)
		} else {
			rl.requests[userID] = validRequests
		}
	}
}
