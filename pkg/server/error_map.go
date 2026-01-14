package server

import (
	"RPCinGo/pkg/circuitbreaker"
	"RPCinGo/pkg/protocol"
	"RPCinGo/pkg/ratelimiter"
	"RPCinGo/pkg/registry"
	"context"
	"errors"
	"fmt"
)

func mapError(err error, service, method string) (code int32, msg string) {
	if errors.Is(err, ratelimiter.ErrRateLimitExceeded) {
		return protocol.ErrorCodeResourceExhausted, "rate limit exceeded"
	}

	if errors.Is(err, circuitbreaker.ErrCircuitOpen) {
		return protocol.ErrorCodeUnavailable,
			fmt.Sprintf("service %s circuit breaker is open", service)
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return protocol.ErrorCodeDeadlineExceeded,
			fmt.Sprintf("method %s.%s execution exceeded deadline", service, method)
	}

	if errors.Is(err, context.Canceled) {
		return protocol.ErrorCodeCanceled,
			fmt.Sprintf("method %s.%s execution canceled", service, method)
	}

	if errors.Is(err, registry.ErrNotFound) {
		return protocol.ErrorCodeNotFound,
			fmt.Sprintf("service %s not found in registry", service)
	}

	return protocol.ErrorCodeInternal,
		fmt.Sprintf("method %s.%s execution failed: %v", service, method, err)
}

func unmapError(resp *protocol.Response) error {
	if resp.IsSuccess() {
		return nil
	}

	err := resp.Error

	switch err.Code {
	case protocol.ErrorCodeResourceExhausted:
		return ratelimiter.ErrRateLimitExceeded

	case protocol.ErrorCodeUnavailable:
		return circuitbreaker.ErrCircuitOpen

	case protocol.ErrorCodeDeadlineExceeded:
		return context.DeadlineExceeded

	case protocol.ErrorCodeCanceled:
		return context.Canceled

	case protocol.ErrorCodeNotFound:
		return fmt.Errorf("%s: %s", err.Message, err.Details)

	case protocol.ErrorCodeInvalidArgument:
		return fmt.Errorf("invalid argument: %s", err.Message)

	default:
		return fmt.Errorf("rpc error [%d]: %s", err.Code, err.Message)
	}
}
