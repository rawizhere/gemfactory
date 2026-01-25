// Package middleware manages request processing flows such as rate limiting and debouncing.
package middleware

import (
	"sync"
	"time"

	"go.uber.org/zap"
)

// RateLimiterInterface defines the contract for request rate limiting.
type RateLimiterInterface interface {
	Allow(userID int64) bool
	AllowRequest(userID int64) bool
	Cleanup()
}

// RateLimiter tracks user requests over a rolling time window.
type RateLimiter struct {
	requests map[int64][]time.Time
	mu       sync.RWMutex
	limit    int
	window   time.Duration
	logger   *zap.Logger
}

var _ RateLimiterInterface = (*RateLimiter)(nil)

func NewRateLimiter(limit int, window time.Duration, logger *zap.Logger) *RateLimiter {
	return &RateLimiter{
		requests: make(map[int64][]time.Time),
		limit:    limit,
		window:   window,
		logger:   logger,
	}
}

func (rl *RateLimiter) AllowRequest(userID int64) bool {
	return rl.allowRequest(userID)
}

func (rl *RateLimiter) Allow(userID int64) bool {
	return rl.allowRequest(userID)
}

func (rl *RateLimiter) allowRequest(userID int64) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-rl.window)

	requests, exists := rl.requests[userID]
	if !exists {
		rl.requests[userID] = []time.Time{now}
		return true
	}

	var validRequests []time.Time
	for _, reqTime := range requests {
		if reqTime.After(windowStart) {
			validRequests = append(validRequests, reqTime)
		}
	}

	if len(validRequests) >= rl.limit {
		rl.logger.Warn("Rate limit exceeded",
			zap.Int64("user_id", userID),
			zap.Int("requests", len(validRequests)),
			zap.Int("limit", rl.limit))
		return false
	}

	validRequests = append(validRequests, now)
	rl.requests[userID] = validRequests

	return true
}

// Cleanup removes expired records.
func (rl *RateLimiter) Cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-rl.window)

	for userID, requests := range rl.requests {
		var validRequests []time.Time
		for _, reqTime := range requests {
			if reqTime.After(windowStart) {
				validRequests = append(validRequests, reqTime)
			}
		}

		if len(validRequests) == 0 {
			delete(rl.requests, userID)
		} else {
			rl.requests[userID] = validRequests
		}
	}
}
