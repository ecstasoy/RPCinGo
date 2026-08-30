// Kunhua Huang 2026

package client

import (
	"context"
	"fmt"
	"github.com/ecstasoy/RPCinGo/pkg/circuitbreaker"
	"github.com/ecstasoy/RPCinGo/pkg/protocol"
	"github.com/ecstasoy/RPCinGo/pkg/ratelimiter"
)

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
