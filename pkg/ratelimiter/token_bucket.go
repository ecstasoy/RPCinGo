// Kunhua Huang 2026

package ratelimiter

import (
	"context"
	"sync"
	"time"
)

// TokenBucketLimiter implements a token-bucket rate limiter.
type TokenBucketLimiter struct {
	capacity int64
	rate     int64

	tokens     int64
	lastUpdate time.Time

	nsRemainder int64 // nanoseconds remainder to carry over for precise refill
	mu          sync.Mutex
}

// NewTokenBucketLimiter returns a token-bucket limiter that refills at rate
// tokens per second with the given capacity.
func NewTokenBucketLimiter(rate, capacity int64) RateLimiter {
	return &TokenBucketLimiter{
		capacity:   capacity,
		rate:       rate,
		tokens:     capacity,
		lastUpdate: time.Now(),
	}
}

// Allow reports whether one token can be consumed immediately.
func (tb *TokenBucketLimiter) Allow(ctx context.Context) bool {
	return tb.AllowN(ctx, 1)
}

// AllowN reports whether n tokens can be consumed immediately.
func (tb *TokenBucketLimiter) AllowN(ctx context.Context, n int) bool {
	if n <= 0 {
		return true
	}

	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill(time.Now())

	if tb.tokens >= int64(n) {
		tb.tokens -= int64(n)
		return true
	}

	return false
}

// Wait blocks until one token becomes available or ctx is canceled.
func (tb *TokenBucketLimiter) Wait(ctx context.Context) error {
	return tb.WaitN(ctx, 1)
}

func (tb *TokenBucketLimiter) refill(now time.Time) {
	elapsed := now.Sub(tb.lastUpdate)
	if elapsed <= 0 {
		return
	}

	elapsedNs := int64(elapsed)
	tb.lastUpdate = now

	nsPerToken := int64(time.Second) / tb.rate
	if nsPerToken <= 0 {
		tb.tokens = tb.capacity
		tb.nsRemainder = 0
		return
	}

	// includes nsRemainder from last time, saving precision
	totalNs := tb.nsRemainder + elapsedNs
	add := totalNs / nsPerToken
	tb.nsRemainder = totalNs % nsPerToken

	if add > 0 {
		tb.tokens += add
		if tb.tokens > tb.capacity {
			tb.tokens = tb.capacity
		}
	}
}

// WaitN blocks until n tokens become available or ctx is canceled.
func (tb *TokenBucketLimiter) WaitN(ctx context.Context, n int) error {
	if n <= 0 {
		return nil
	}

	if int64(n) > tb.capacity {
		return ErrInvalidRequest
	}

	for {
		tb.mu.Lock()
		now := time.Now()
		tb.refill(now)

		if tb.tokens >= int64(n) {
			tb.tokens -= int64(n)
			tb.mu.Unlock()
			return nil
		}

		neededTokens := int64(n) - tb.tokens
		nsPerToken := int64(time.Second) / tb.rate
		waitNs := neededTokens * nsPerToken

		if waitNs < int64(time.Microsecond) {
			waitNs = int64(time.Microsecond)
		}

		tb.mu.Unlock()

		timer := time.NewTimer(time.Duration(waitNs))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
			// continue to try
		}
	}
}

// Name returns the limiter name.
func (tb *TokenBucketLimiter) Name() string {
	return "token-bucket"
}
