package middleware

import (
	"slices"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2/expirable"
)

type grokUserState struct {
	requests []time.Time
	notified bool
}

type GrokLimiter struct {
	mu    sync.Mutex
	cache *lru.LRU[int64, *grokUserState]
}

func NewGrokLimiter() *GrokLimiter {
	return &GrokLimiter{
		cache: lru.NewLRU[int64, *grokUserState](maxTrackedUsers, nil, 10*time.Minute),
	}
}

func (l *GrokLimiter) Check(userID int64, limit int, window time.Duration) (bool, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-window)

	state, ok := l.cache.Get(userID)
	if !ok || state == nil {
		state = &grokUserState{}
		l.cache.Add(userID, state)
	}

	state.requests = slices.DeleteFunc(state.requests, func(reqTime time.Time) bool {
		return !reqTime.After(windowStart)
	})

	if len(state.requests) < limit {
		state.requests = append(state.requests, now)
		state.notified = false
		return true, false
	}

	if !state.notified {
		state.notified = true
		return false, true
	}

	return false, false
}
