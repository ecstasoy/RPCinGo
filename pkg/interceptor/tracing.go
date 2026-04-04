package interceptor

import (
	"RPCinGo/pkg/protocol"
	"RPCinGo/pkg/tracing"
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

func TracingClient() Interceptor {
	return func(ctx context.Context, req *protocol.Request, invoker Invoker) (any, error) {
		ctx, span := tracing.Start(ctx, "rpc.client/"+req.Service+"/"+req.Method)
		defer span.End()

		span.SetAttributes(
			attribute.String("rpc.service", req.Service),
			attribute.String("rpc.method", req.Method),
		)

		otel.GetTextMapPropagator().Inject(ctx, metadataCarrier(req.Metadata))

		result, err := invoker(ctx, req)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		return result, err
	}
}

func TracingServer() Interceptor {
	return func(ctx context.Context, req *protocol.Request, invoker Invoker) (any, error) {
		// Extract trace context propagated from client
		carrier := metadataCarrier(req.Metadata)
		ctx = otel.GetTextMapPropagator().Extract(ctx, carrier)

		ctx, span := tracing.Start(ctx, "rpc.server/"+req.Service+"/"+req.Method)
		defer span.End()

		req.SetMetadata(protocol.MetaKeySpanID, span.SpanContext().SpanID().String())
		span.SetAttributes(
			attribute.String("rpc.service", req.Service),
			attribute.String("rpc.method", req.Method),
		)

		result, err := invoker(ctx, req)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		return result, err
	}
}

type metadataCarrier protocol.Metadata

func (m metadataCarrier) Get(key string) string {
	result, _ := protocol.Metadata(m).Get(key)
	return result
}

func (m metadataCarrier) Set(key, value string) {
	protocol.Metadata(m).Set(key, value)
}

func (m metadataCarrier) Keys() []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
