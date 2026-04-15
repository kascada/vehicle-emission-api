package cache

import (
	"context"
	"testing"
	"time"
)

func TestRateLimiter_AllowWithinLimit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rl := NewRateLimiter(5, time.Minute, ctx)

	for i := 0; i < 5; i++ {
		if !rl.Allow("user@test.com") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
}

func TestRateLimiter_BlocksOverLimit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rl := NewRateLimiter(3, time.Minute, ctx)

	for i := 0; i < 3; i++ {
		rl.Allow("user@test.com")
	}

	if rl.Allow("user@test.com") {
		t.Error("4th request should be blocked")
	}
}

func TestRateLimiter_SeparateKeys(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rl := NewRateLimiter(1, time.Minute, ctx)

	if !rl.Allow("a@test.com") {
		t.Error("first key should be allowed")
	}
	if !rl.Allow("b@test.com") {
		t.Error("second key should be allowed (separate counter)")
	}
	if rl.Allow("a@test.com") {
		t.Error("first key should now be blocked")
	}
}

func TestRateLimiter_WindowResets(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rl := NewRateLimiter(1, 50*time.Millisecond, ctx)

	if !rl.Allow("user@test.com") {
		t.Fatal("first request should be allowed")
	}
	if rl.Allow("user@test.com") {
		t.Fatal("second request should be blocked")
	}

	time.Sleep(60 * time.Millisecond)

	if !rl.Allow("user@test.com") {
		t.Error("request after window reset should be allowed")
	}
}
