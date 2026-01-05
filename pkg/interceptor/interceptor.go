// Kunhua Huang 2026

package interceptor

import (
	"RPCinGo/pkg/protocol"
	"context"
)

type Invoker func(ctx context.Context, req *protocol.Request) (any, error)

type Interceptor func(ctx context.Context, req *protocol.Request, invoker Invoker) (any, error)

type Chain struct {
	interceptors []Interceptor
}

func NewChain(interceptor ...Interceptor) *Chain {
	return &Chain{interceptors: interceptor}
}

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
