package middleware

import (
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
)

// limiterStateTTL drops per-user state after this long without requests.
const limiterStateTTL = 10 * time.Minute

type limiterState struct {
	requests []time.Time
	notified bool
	lastSeen time.Time
}

// Limiter is a per-user sliding-window rate limiter with an optional notify-once signal on refusal.
type Limiter struct {
	mu        sync.Mutex
	users     map[int64]*limiterState
	lastSweep time.Time
}

func NewLimiter() *Limiter {
	return &Limiter{users: map[int64]*limiterState{}}
}

// Check prunes requests outside the window, records the request when under the limit,
// and reports shouldNotify=true only for the first refusal in a row.
func (l *Limiter) Check(userID int64, limit int, window time.Duration) (allowed, shouldNotify bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-window)

	if now.Sub(l.lastSweep) > limiterStateTTL {
		maps.DeleteFunc(l.users, func(_ int64, st *limiterState) bool { return now.Sub(st.lastSeen) > limiterStateTTL })
		l.lastSweep = now
	}
	state, ok := l.users[userID]
	if !ok {
		state = &limiterState{}
		l.users[userID] = state
	}
	state.lastSeen = now

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
