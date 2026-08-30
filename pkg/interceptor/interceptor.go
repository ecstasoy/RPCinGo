// Kunhua Huang 2026

package interceptor

import (
	"github.com/ecstasoy/RPCinGo/pkg/protocol"
	"context"
)

// Invoker is the terminal function wrapped by one or more interceptors.
type Invoker func(ctx context.Context, req *protocol.Request) (any, error)

// Interceptor wraps an Invoker to add cross-cutting behavior around one
// request.
type Interceptor func(ctx context.Context, req *protocol.Request, invoker Invoker) (any, error)

// Chain stores interceptors and executes them in registration order.
type Chain struct {
	interceptors []Interceptor
}

// NewChain returns a Chain built from interceptor.
func NewChain(interceptor ...Interceptor) *Chain {
	return &Chain{interceptors: interceptor}
}

// Intercept executes the interceptor chain around invoker.
func (ic *Chain) Intercept(ctx context.Context, req *protocol.Request, invoker Invoker) (any, error) {
	if len(ic.interceptors) == 0 {
		return invoker(ctx, req)
	}

	return ic.buildChain(invoker)(ctx, req)
}

func (ic *Chain) buildChain(invoker Invoker) Invoker {
	for i := len(ic.interceptors) - 1; i >= 0; i-- {
		next := invoker
		interceptor := ic.interceptors[i]

		invoker = func(ctx context.Context, req *protocol.Request) (any, error) {
			return interceptor(ctx, req, next)
		}
	}

	return invoker
}
