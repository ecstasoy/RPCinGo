//Kunhua Huang 2026

package interceptor

import (
	"RPCinGo/pkg/protocol"
	"RPCinGo/pkg/ratelimiter"
	"context"
)

// RateLimit returns an interceptor that rejects requests when limiter denies
// them.
func RateLimit(limiter ratelimiter.RateLimiter) Interceptor {
	return func(ctx context.Context, req *protocol.Request, invoker Invoker) (any, error) {
		if !limiter.Allow(ctx) {
			return nil, ratelimiter.ErrRateLimitExceeded
		}

		return invoker(ctx, req)
	}
}
