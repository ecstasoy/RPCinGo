// Kunhua Huang 2026

package interceptor

import (
	"context"
	"fmt"
	"runtime/debug"
)

func Recovery() Interceptor {
	return func(ctx context.Context, req interface{}, invoker Invoker) (resp interface{}, err error) {
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
