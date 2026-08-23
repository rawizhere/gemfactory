package middleware

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestDebouncerShouldProcess(t *testing.T) {
	d := NewDebouncer(50*time.Millisecond, zap.NewNop())

	if !d.ShouldProcess(1, "msg:month") {
		t.Error("first call must pass")
	}
	if d.ShouldProcess(1, "msg:month") {
		t.Error("repeated call within interval must be dropped")
	}
	if !d.ShouldProcess(1, "msg:clip") {
		t.Error("different action for same user must pass")
	}
	if !d.ShouldProcess(2, "msg:month") {
		t.Error("same action for different user must pass")
	}

	time.Sleep(60 * time.Millisecond)
	if !d.ShouldProcess(1, "msg:month") {
		t.Error("call after interval must pass")
	}
}

func TestDebouncerEntriesExpire(t *testing.T) {
	d := NewDebouncer(10*time.Millisecond, zap.NewNop())
	d.ShouldProcess(1, "a")
	d.ShouldProcess(2, "b")

	time.Sleep(30 * time.Millisecond)

	if _, ok := d.lastProcess.Get("1:a"); ok {
		t.Errorf("expected expired entry to be gone")
	}
	if _, ok := d.lastProcess.Get("2:b"); ok {
		t.Errorf("expected expired entry to be gone")
	}
}

func TestRateLimiterAllow(t *testing.T) {
	rl := NewRateLimiter(3, 50*time.Millisecond, zap.NewNop())

	for i := 0; i < 3; i++ {
		if !rl.Allow(1) {
			t.Fatalf("request %d must be allowed", i+1)
		}
	}
	if rl.Allow(1) {
		t.Error("4th request within window must be rejected")
	}
	if !rl.Allow(2) {
		t.Error("other user must not be affected")
	}

	time.Sleep(60 * time.Millisecond)
	if !rl.Allow(1) {
		t.Error("request in a fresh window must be allowed")
	}
}

func TestRateLimiterEntriesExpire(t *testing.T) {
	rl := NewRateLimiter(2, 20*time.Millisecond, zap.NewNop())
	rl.Allow(1)
	rl.Allow(1)

	time.Sleep(40 * time.Millisecond)

	if _, ok := rl.requests.Get(1); ok {
		t.Errorf("expected expired user state to be gone")
	}
}
