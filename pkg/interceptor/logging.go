// Kunhua Huang 2026

package interceptor

import (
	"context"
	"time"

	"github.com/ecstasoy/RPCinGo/pkg/logger"
	"github.com/ecstasoy/RPCinGo/pkg/protocol"
	"github.com/ecstasoy/RPCinGo/pkg/tracing"
)

// Logging returns an interceptor that logs request outcome, duration, and trace
// ID.
func Logging(l logger.Logger) Interceptor {
	if l == nil {
		l = logger.New()
	}

	return func(ctx context.Context, req *protocol.Request, invoker Invoker) (any, error) {
		start := time.Now()
		traceID := tracing.TraceID(ctx)

		resp, err := invoker(ctx, req)

		dur := time.Since(start)
		if err != nil {
			l.Error("rpc call failed",
				"service", req.Service, "method", req.Method,
				"trace", traceID, "duration", dur, "error", err)
		} else {
			l.Info("rpc call ok",
				"service", req.Service, "method", req.Method,
				"trace", traceID, "duration", dur)
		}

		return resp, err
	}
}
