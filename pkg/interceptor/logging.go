// Kunhua Huang 2026

package interceptor

import (
	"RPCinGo/pkg/protocol"
	"context"
	"fmt"
	"time"
)

type Logger interface {
	Infof(format string, args ...interface{})
	Errorf(format string, args ...interface{})
}

type defaultLogger struct{}

func (l *defaultLogger) Infof(format string, args ...interface{}) {
	fmt.Printf("[INFO] "+format+"\n", args...)
}

func (l *defaultLogger) Errorf(format string, args ...interface{}) {
	fmt.Printf("[ERROR] "+format+"\n", args...)
}

func Logging(logger Logger) Interceptor {
	if logger == nil {
		logger = &defaultLogger{}
	}

	return func(ctx context.Context, req *protocol.Request, invoker Invoker) (any, error) {
		start := time.Now()

		var service, method string
		logger.Infof("→ RPC call: [%s.%s]", service, method)

		resp, err := invoker(ctx, req)

		duration := time.Since(start)

		if err != nil {
			logger.Errorf("✗ RPC call: [%s.%s] failed in %v: %v", service, method, duration, err)
		} else {
			logger.Infof("✓ RPC call: [%s.%s] succeeded in %v", service, method, duration)
		}

		return resp, err
	}
}
