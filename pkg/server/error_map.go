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

// ErrInvalidArgument is the sentinel a handler may return to signal a malformed
// request. It exists so the InvalidArgument protocol code has a Go counterpart
// and the mapping stays symmetric in both directions.
var ErrInvalidArgument = errors.New("invalid argument")

// errorMapping is one row of the single source of truth that ties a Go sentinel
// error to its protocol error code. mapError and unmapError both derive from
// errorTable, so a new code is added in exactly one place and the two
// directions cannot drift out of sync.
type errorMapping struct {
	code     int32
	sentinel error
	// message builds the server-side human message from call context. It is
	// only consulted on the encode (server) side.
	message func(service, method string) string
}

var errorTable = []errorMapping{
	{
		code:     protocol.ErrorCodeResourceExhausted,
		sentinel: ratelimiter.ErrRateLimitExceeded,
		message:  func(service, method string) string { return "rate limit exceeded" },
	},
	{
		code:     protocol.ErrorCodeUnavailable,
		sentinel: circuitbreaker.ErrCircuitOpen,
		message: func(service, method string) string {
			return fmt.Sprintf("service %s circuit breaker is open", service)
		},
	},
	{
		code:     protocol.ErrorCodeDeadlineExceeded,
		sentinel: context.DeadlineExceeded,
		message: func(service, method string) string {
			return fmt.Sprintf("method %s.%s execution exceeded deadline", service, method)
		},
	},
	{
		code:     protocol.ErrorCodeCanceled,
		sentinel: context.Canceled,
		message: func(service, method string) string {
			return fmt.Sprintf("method %s.%s execution canceled", service, method)
		},
	},
	{
		code:     protocol.ErrorCodeNotFound,
		sentinel: registry.ErrNotFound,
		message: func(service, method string) string {
			return fmt.Sprintf("service %s not found in registry", service)
		},
	},
	{
		code:     protocol.ErrorCodeInvalidArgument,
		sentinel: ErrInvalidArgument,
		message:  func(service, method string) string { return "invalid argument" },
	},
}

// sentinelForCode returns the Go sentinel registered for a protocol code, or nil
// when the code carries no framework sentinel.
func sentinelForCode(code int32) error {
	for i := range errorTable {
		if errorTable[i].code == code {
			return errorTable[i].sentinel
		}
	}
	return nil
}

// mapError converts a handler error into a protocol error code and message by
// matching it against the sentinels in errorTable. Unrecognized errors map to
// Internal.
func mapError(err error, service, method string) (code int32, msg string) {
	for i := range errorTable {
		if errors.Is(err, errorTable[i].sentinel) {
			return errorTable[i].code, errorTable[i].message(service, method)
		}
	}

	return protocol.ErrorCodeInternal,
		fmt.Sprintf("method %s.%s execution failed: %v", service, method, err)
}

// unmapError reverses mapError on the client side. When the response carries a
// framework code, it returns that code's sentinel (wrapped with the server
// message when one is present) so callers can match it with errors.Is.
// Unrecognized codes degrade to a plain formatted error.
func unmapError(resp *protocol.Response) error {
	if resp.IsSuccess() {
		return nil
	}

	respErr := resp.Error

	if sentinel := sentinelForCode(respErr.Code); sentinel != nil {
		if respErr.Details != "" {
			return fmt.Errorf("%w: %s", sentinel, respErr.Details)
		}
		return sentinel
	}

	return fmt.Errorf("rpc error [%d]: %s", respErr.Code, respErr.Message)
}
