package middleware

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestDebouncerShouldProcess(t *testing.T) {
	d := NewDebouncer(50*time.Millisecond, zap.NewNop())

	require.True(t, d.ShouldProcess(1, "msg:month"), "first call must pass")
	require.False(t, d.ShouldProcess(1, "msg:month"), "repeated call within interval must be dropped")
	require.True(t, d.ShouldProcess(1, "msg:clip"), "different action for same user must pass")
	require.True(t, d.ShouldProcess(2, "msg:month"), "same action for different user must pass")

	time.Sleep(60 * time.Millisecond)
	require.True(t, d.ShouldProcess(1, "msg:month"), "call after interval must pass")
}

func TestDebouncerEntriesExpire(t *testing.T) {
	d := NewDebouncer(10*time.Millisecond, zap.NewNop())
	d.ShouldProcess(1, "a")
	d.ShouldProcess(2, "b")

	time.Sleep(30 * time.Millisecond)

	_, ok := d.lastProcess.Get("1:a")
	require.False(t, ok, "expected expired entry to be gone")
	_, ok = d.lastProcess.Get("2:b")
	require.False(t, ok, "expected expired entry to be gone")
}

func TestRateLimiterAllow(t *testing.T) {
	rl := NewRateLimiter(3, 50*time.Millisecond, zap.NewNop())

	for i := 0; i < 3; i++ {
		require.True(t, rl.Allow(1), "request %d must be allowed", i+1)
	}
	require.False(t, rl.Allow(1), "4th request within window must be rejected")
	require.True(t, rl.Allow(2), "other user must not be affected")

	time.Sleep(60 * time.Millisecond)
	require.True(t, rl.Allow(1), "request in a fresh window must be allowed")
}

func TestRateLimiterEntriesExpire(t *testing.T) {
	rl := NewRateLimiter(2, 20*time.Millisecond, zap.NewNop())
	rl.Allow(1)
	rl.Allow(1)

	time.Sleep(40 * time.Millisecond)

	_, ok := rl.requests.Get(1)
	require.False(t, ok, "expected expired user state to be gone")
}
