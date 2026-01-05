// Kunhua Huang 2026

package interceptor

import (
	"RPCinGo/pkg/protocol"
	"context"
	"fmt"
	"runtime/debug"
)

func Recovery() Interceptor {
	return func(ctx context.Context, req *protocol.Request, invoker Invoker) (resp any, err error) {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				err = fmt.Errorf("panic recovered: %v\nstack:\n%s", r, stack)
				resp = nil
			}
		}()

		return invoker(ctx, req)
	}
}
