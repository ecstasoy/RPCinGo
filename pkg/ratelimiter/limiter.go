// Kunhua Huang 2026

package ratelimiter

import (
	"context"
	"errors"
)

var (
	ErrRateLimitExceeded = errors.New("rate limit exceeded")
	ErrInvalidRequest    = errors.New("invalid request for rate limiter")
)

type RateLimiter interface {
	Allow(ctx context.Context) bool
	AllowN(ctx context.Context, n int) bool
	Wait(ctx context.Context) error
	Name() string
}
