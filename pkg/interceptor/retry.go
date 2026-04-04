// Kunhua Huang 2026

package interceptor

import (
	"context"
	"time"

	"RPCinGo/pkg/protocol"
)

// Retry returns an interceptor that retries the RPC call on transient failures.
//
//   - maxRetries: maximum number of additional attempts after the first (0 = no retry).
//   - interval:   wait duration between attempts; applies a simple fixed delay.
//
// Only errors that are safe to retry are retried (network errors and a subset of
// protocol error codes: Unavailable, DeadlineExceeded, ResourceExhausted).
// Application errors (NotFound, InvalidArgument, PermissionDenied, etc.) are
// returned immediately without retrying.
func Retry(maxRetries int, interval time.Duration) Interceptor {
	return func(ctx context.Context, req *protocol.Request, invoker Invoker) (any, error) {
		var lastErr error
		for attempt := 0; attempt <= maxRetries; attempt++ {
			if attempt > 0 {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(interval):
				}
			}

			result, err := invoker(ctx, req)
			if err == nil {
				return result, nil
			}

			if !isRetryable(err) {
				return nil, err
			}
			lastErr = err
		}
		return nil, lastErr
	}
}

// isRetryable returns true only for errors that may resolve on a subsequent attempt.
// Application-level errors (wrong args, missing method, auth failures) will not
// succeed on retry, so they are excluded to avoid wasted round-trips.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	protoErr, ok := err.(*protocol.Error)
	if !ok {
		// Non-protocol errors are network/IO errors — safe to retry.
		return true
	}
	switch protoErr.Code {
	case protocol.ErrorCodeUnavailable,
		protocol.ErrorCodeDeadlineExceeded,
		protocol.ErrorCodeResourceExhausted:
		return true
	default:
		return false
	}
}
