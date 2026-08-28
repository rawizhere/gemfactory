package middleware

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGrokLimiter_Check(t *testing.T) {
	limiter := NewGrokLimiter()
	userID := int64(12345)
	limit := 3
	window := 100 * time.Millisecond

	// 1st request - allowed
	allowed, notify := limiter.Check(userID, limit, window)
	assert.True(t, allowed)
	assert.False(t, notify)

	// 2nd request - allowed
	allowed, notify = limiter.Check(userID, limit, window)
	assert.True(t, allowed)
	assert.False(t, notify)

	// 3rd request - allowed
	allowed, notify = limiter.Check(userID, limit, window)
	assert.True(t, allowed)
	assert.False(t, notify)

	// 4th request - rejected, first notification
	allowed, notify = limiter.Check(userID, limit, window)
	assert.False(t, allowed)
	assert.True(t, notify)

	// 5th request - rejected, silent
	allowed, notify = limiter.Check(userID, limit, window)
	assert.False(t, allowed)
	assert.False(t, notify)

	// Wait for window to expire
	time.Sleep(120 * time.Millisecond)

	// Next request after expiry - allowed
	allowed, notify = limiter.Check(userID, limit, window)
	assert.True(t, allowed)
	assert.False(t, notify)
}

func TestGrokLimiter_Concurrent(t *testing.T) {
	limiter := NewGrokLimiter()
	userID := int64(99999)
	limit := 5
	window := time.Second

	var wg sync.WaitGroup
	var allowedCount, notifyCount, silentCount int
	var mu sync.Mutex

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed, notify := limiter.Check(userID, limit, window)
			mu.Lock()
			switch {
			case allowed:
				allowedCount++
			case notify:
				notifyCount++
			default:
				silentCount++
			}
			mu.Unlock()
		}()
	}

	wg.Wait()
	assert.Equal(t, limit, allowedCount)
	assert.Equal(t, 1, notifyCount)
	assert.Equal(t, 20-limit-1, silentCount)
}
