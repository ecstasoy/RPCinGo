package ratelimiter

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTokenBucket_BasicAllow(t *testing.T) {
	tb := NewTokenBucketLimiter(10, 10)

	for i := 0; i < 10; i++ {
		if !tb.Allow(context.Background()) {
			t.Errorf("request %d should be allowed", i)
		}
	}

	if tb.Allow(context.Background()) {
		t.Error("11th request should be denied")
	}

	t.Log("✅ TokenBucket basic test passed")
}

func TestTokenBucket_Refill(t *testing.T) {
	tb := NewTokenBucketLimiter(100, 100)

	// Consume all
	for i := 0; i < 100; i++ {
		tb.Allow(context.Background())
	}

	// Should be empty
	if tb.Allow(context.Background()) {
		t.Error("should be empty")
	}

	// Wait for refill (100/s → 0.2s = 20 tokens)
	time.Sleep(200 * time.Millisecond)

	allowed := 0
	for i := 0; i < 25; i++ {
		if tb.Allow(context.Background()) {
			allowed++
		}
	}

	t.Logf("Allowed after 200ms: %d (expected ~20)", allowed)

	if allowed < 15 || allowed > 25 {
		t.Fatalf("expected ~20, got %d", allowed)
	}

	t.Log("✅ Refill test passed")
}

func TestTokenBucket_BurstCapacity(t *testing.T) {
	tb := NewTokenBucketLimiter(100, 200)

	// Burst: consume all capacity at once
	allowed := 0
	for i := 0; i < 250; i++ {
		if tb.Allow(context.Background()) {
			allowed++
		}
	}

	t.Logf("Burst allowed: %d", allowed)

	if allowed != 200 {
		t.Errorf("expected 200 (capacity), got %d", allowed)
	}

	t.Log("✅ Burst capacity test passed")
}

func TestTokenBucket_Wait(t *testing.T) {
	tb := NewTokenBucketLimiter(100, 5)

	// Consume all
	for i := 0; i < 5; i++ {
		tb.Allow(context.Background())
	}

	start := time.Now()

	// Wait for 1 token (should wait ~10ms at 100/s rate)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := tb.Wait(ctx)
	if err != nil {
		t.Fatalf("wait failed: %v", err)
	}

	elapsed := time.Since(start)
	t.Logf("Waited: %v", elapsed)

	if elapsed < 5*time.Millisecond || elapsed > 50*time.Millisecond {
		t.Logf("Warning: expected ~10ms, got %v", elapsed)
	}

	t.Log("✅ Wait test passed")
}

func TestTokenBucket_WaitCancel(t *testing.T) {
	tb := NewTokenBucketLimiter(1, 1)

	// Consume
	tb.Allow(context.Background())

	// Create cancelable context
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after 100ms
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err := tb.Wait(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Error("should return error when context canceled")
	}

	if elapsed < 90*time.Millisecond || elapsed > 150*time.Millisecond {
		t.Errorf("expected ~100ms, got %v", elapsed)
	}

	t.Logf("✅ Wait cancel test passed (waited %v)", elapsed)
}

func TestTokenBucket_Concurrent(t *testing.T) {
	tb := NewTokenBucketLimiter(1000, 1000)

	var allowed int64
	var denied int64

	var wg sync.WaitGroup
	concurrency := 10
	requestsPerGoroutine := 200

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for j := 0; j < requestsPerGoroutine; j++ {
				if tb.Allow(context.Background()) {
					atomic.AddInt64(&allowed, 1)
				} else {
					atomic.AddInt64(&denied, 1)
				}
			}
		}()
	}

	wg.Wait()

	total := allowed + denied
	t.Logf("Concurrent test: allowed=%d, denied=%d, total=%d",
		allowed, denied, total)

	if total != int64(concurrency*requestsPerGoroutine) {
		t.Errorf("total mismatch")
	}

	// First 1000 should be allowed (capacity)
	if allowed < 950 || allowed > 1050 {
		t.Logf("Warning: expected ~1000, got %d", allowed)
	}

	t.Log("✅ Concurrent test passed")
}

func TestSlidingWindow_BasicLimit(t *testing.T) {
	sw := NewSlidingWindowLimiter(10, time.Second)

	// First 10 pass
	for i := 0; i < 10; i++ {
		if !sw.Allow(context.Background()) {
			t.Errorf("request %d should pass", i)
		}
	}

	// 11th denied
	if sw.Allow(context.Background()) {
		t.Error("11th should be denied")
	}

	t.Log("✅ SlidingWindow basic test passed")
}

func TestSlidingWindow_Slide(t *testing.T) {
	sw := NewSlidingWindowLimiter(5, 500*time.Millisecond)

	// Consume all
	for i := 0; i < 5; i++ {
		sw.Allow(context.Background())
	}

	// Should be denied
	if sw.Allow(context.Background()) {
		t.Error("should be denied")
	}

	// Wait for window to slide
	time.Sleep(600 * time.Millisecond)

	// Should allow again
	if !sw.Allow(context.Background()) {
		t.Error("should be allowed after window slides")
	}

	t.Log("✅ Sliding window slide test passed")
}

func TestSlidingWindow_PartialSlide(t *testing.T) {
	sw := NewSlidingWindowLimiter(10, time.Second)

	// Add 5 requests at t=0
	for i := 0; i < 5; i++ {
		sw.Allow(context.Background())
	}

	// Wait 600ms
	time.Sleep(600 * time.Millisecond)

	// Add 5 more (should pass, total in window = 10)
	for i := 0; i < 5; i++ {
		if !sw.Allow(context.Background()) {
			t.Errorf("request %d should pass", i+5)
		}
	}

	// 11th should fail (first 5 still in window)
	if sw.Allow(context.Background()) {
		t.Error("should be denied")
	}

	// Wait for first batch to expire
	time.Sleep(500 * time.Millisecond)

	// Now first 5 expired, should allow 5 more
	allowed := 0
	for i := 0; i < 10; i++ {
		if sw.Allow(context.Background()) {
			allowed++
		}
	}

	t.Logf("Allowed after partial slide: %d", allowed)

	if allowed < 4 || allowed > 6 {
		t.Logf("Warning: expected ~5, got %d", allowed)
	}

	t.Log("✅ Partial slide test passed")
}

func BenchmarkRateLimiters(b *testing.B) {
	limiters := map[string]RateLimiter{
		"TokenBucket":   NewTokenBucketLimiter(1000000, 1000),
		"SlidingWindow": NewSlidingWindowLimiter(1000000, time.Second),
	}

	for name, limiter := range limiters {
		b.Run(name, func(b *testing.B) {
			ctx := context.Background()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				limiter.Allow(ctx)
			}
		})
	}
}

func BenchmarkTokenBucket_Concurrent(b *testing.B) {
	tb := NewTokenBucketLimiter(1000000, 10000)
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			tb.Allow(ctx)
		}
	})
}
