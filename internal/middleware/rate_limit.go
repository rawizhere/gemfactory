package middleware

import (
	"slices"
	"time"

	lru "github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	"go.uber.org/zap"
)

// maxTrackedUsers bounds the per-user state kept in memory.
const maxTrackedUsers = 10_000

type RateLimiter struct {
	requests *lru.LRU[int64, []time.Time]
	limit    int
	window   time.Duration
	logger   *zap.Logger
}

func NewRateLimiter(limit int, window time.Duration, logger *zap.Logger) *RateLimiter {
	return &RateLimiter{
		requests: lru.NewLRU[int64, []time.Time](maxTrackedUsers, nil, window),
		limit:    limit,
		window:   window,
		logger:   logger,
	}
}

func (rl *RateLimiter) Allow(userID int64) bool {
	now := time.Now()
	windowStart := now.Add(-rl.window)

	var requests []time.Time
	if cached, ok := rl.requests.Peek(userID); ok {
		requests = slices.Clone(cached)
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
	// Add refreshes the entry so its TTL restarts with the newest request.
	rl.requests.Add(userID, requests)

	return true
}

// RateLimit drops message updates from users exceeding the request quota.
func RateLimit(rl *RateLimiter) th.Handler {
	return func(ctx *th.Context, update telego.Update) error {
		if update.Message != nil && update.Message.From != nil && !rl.Allow(update.Message.From.ID) {
			return nil
		}
		return ctx.Next(update)
	}
}
