// Kunhua Huang 2026

package interceptor

import (
	"RPCinGo/pkg/protocol"
	"RPCinGo/pkg/tracing"
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

		service, method := req.Service, req.Method
		traceID := tracing.TraceID(ctx)
		logger.Infof("→ RPC call: [%s.%s] trace=%s", service, method, traceID)

		resp, err := invoker(ctx, req)

		duration := time.Since(start)

		if err != nil {
			logger.Errorf("✗ RPC call: [%s.%s] trace=%s failed in %v: %v", service, method, traceID, duration, err)
		} else {
			logger.Infof("✓ RPC call: [%s.%s] trace=%s succeeded in %v", service, method, traceID, duration)
		}

		return resp, err
	}
}
