package middleware

import (
	"slices"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
)

// maxTrackedUsers bounds the per-user state kept in memory.
const maxTrackedUsers = 10_000

type limiterState struct {
	requests []time.Time
	notified bool
}

// Limiter is a per-user sliding-window rate limiter with an optional notify-once signal on refusal.
type Limiter struct {
	mu    sync.Mutex
	cache *lru.LRU[int64, *limiterState]
}

func NewLimiter() *Limiter {
	return &Limiter{cache: lru.NewLRU[int64, *limiterState](maxTrackedUsers, nil, 10*time.Minute)}
}

// Check prunes requests outside the window, records the request when under the limit,
// and reports shouldNotify=true only for the first refusal in a row.
func (l *Limiter) Check(userID int64, limit int, window time.Duration) (allowed, shouldNotify bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-window)

	state, ok := l.cache.Get(userID)
	if !ok || state == nil {
		state = &limiterState{}
		l.cache.Add(userID, state)
	}

	state.requests = slices.DeleteFunc(state.requests, func(t time.Time) bool { return !t.After(windowStart) })

	if len(state.requests) >= limit {
		first := !state.notified
		state.notified = true
		return false, first
	}

	state.requests = append(state.requests, now)
	state.notified = false
	return true, false
}

// RateLimit drops message updates from users exceeding the request quota.
func RateLimit(l *Limiter, limit int, window time.Duration) th.Handler {
	return func(ctx *th.Context, update telego.Update) error {
		if update.Message != nil && update.Message.From != nil {
			if allowed, _ := l.Check(update.Message.From.ID, limit, window); !allowed {
				return nil
			}
		}
		return ctx.Next(update)
	}
}
