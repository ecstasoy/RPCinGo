package client

import "context"

// hashKeyCtxKey is the private context key under which a load-balancer affinity
// key is carried into Call.
type hashKeyCtxKey struct{}

// WithHashKey returns a context that pins the call to a consistent-hash affinity
// key. When the client uses a consistent-hash load balancer, requests carrying
// the same key are routed to the same instance. It is a no-op for balancers that
// do not support option-based selection.
func WithHashKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, hashKeyCtxKey{}, key)
}

// hashKeyFromContext reports the affinity key carried by ctx, if any.
func hashKeyFromContext(ctx context.Context) (string, bool) {
	key, ok := ctx.Value(hashKeyCtxKey{}).(string)
	return key, ok && key != ""
}
