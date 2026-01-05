// Kunhua Huang 2026

package ratelimiter

import (
	"context"
	"sync"
	"time"
)

type SlidingWindowLimiter struct {
	limit  int64
	window time.Duration

	requests []time.Time
	mu       sync.Mutex
}

func NewSlidingWindowLimiter(limit int64, window time.Duration) RateLimiter {
	return &SlidingWindowLimiter{
		limit:    limit,
		window:   window,
		requests: make([]time.Time, 0),
	}
}

func (swl *SlidingWindowLimiter) Allow(ctx context.Context) bool {
	return swl.AllowN(ctx, 1)
}

func (swl *SlidingWindowLimiter) AllowN(ctx context.Context, n int) bool {
	swl.mu.Lock()
	defer swl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-swl.window)

	validRequests := make([]time.Time, 0)
	for _, t := range swl.requests {
		if t.After(windowStart) {
			validRequests = append(validRequests, t)
		}
	}

	swl.requests = validRequests

	if int64(len(swl.requests))+int64(n) > swl.limit {
		return false
	}

	for i := 0; i < n; i++ {
		swl.requests = append(swl.requests, now)
	}

	return true
}

func (swl *SlidingWindowLimiter) Wait(ctx context.Context) error {
	for {
		if swl.Allow(ctx) {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func (swl *SlidingWindowLimiter) Name() string {
	return "sliding-window"
}
