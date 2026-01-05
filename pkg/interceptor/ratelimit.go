//Kunhua Huang 2026

package interceptor

import (
	"RPCinGo/pkg/ratelimiter"
	"context"
)

func RateLimit(limiter ratelimiter.RateLimiter) Interceptor {
	return func(ctx context.Context, req interface{}, invoker Invoker) (interface{}, error) {
		if !limiter.Allow(ctx) {
			return nil, ratelimiter.ErrRateLimitExceeded
		}

		return invoker(ctx, req)
	}
}
