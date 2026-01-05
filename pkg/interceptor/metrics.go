// Kunhua Huang 2026

package interceptor

import (
	"RPCinGo/pkg/protocol"
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	rpcCallsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rpc_calls_total",
			Help: "Total number of RPC calls",
		},
		[]string{"service", "method", "status"},
	)
	rpcDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rpc_duration_seconds",
			Help:    "Duration of RPC calls in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"service", "method"},
	)
)

func init() {
	prometheus.MustRegister(rpcCallsTotal)
	prometheus.MustRegister(rpcDuration)
}

func Metrics() Interceptor {
	return func(ctx context.Context, req *protocol.Request, invoker Invoker) (any, error) {
		start := time.Now()

		var service, method string

		resp, err := invoker(ctx, req)

		duration := time.Since(start).Seconds()

		status := "success"
		if err != nil {
			status = "error"
		}

		rpcCallsTotal.WithLabelValues(service, method, status).Inc()
		rpcDuration.WithLabelValues(service, method).Observe(duration)

		return resp, err
	}
}
