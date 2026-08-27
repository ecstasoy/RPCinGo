package server

import (
	"testing"
	"time"

	"RPCinGo/pkg/transport"
)

// TestHandlerTimeoutReachesTransport verifies that the server-level
// WithHandlerTimeout option is translated through to the underlying transport,
// closing the gap where HandlerTimeout was previously unreachable from the
// server constructor.
func TestHandlerTimeoutReachesTransport(t *testing.T) {
	srv := NewServer(WithHandlerTimeout(7 * time.Second))

	got := srv.Transport.Options().HandlerTimeout
	if got != 7*time.Second {
		t.Fatalf("transport HandlerTimeout = %v, want 7s", got)
	}
}

// TestTransportOptionsPassThrough verifies that raw transport options forwarded
// via WithTransportOptions reach the transport, so every transport knob is
// reachable through the single server constructor.
func TestTransportOptionsPassThrough(t *testing.T) {
	srv := NewServer(
		WithTransportOptions(
			transport.WithMaxRequestBodySize(4242),
			transport.WithServerBufferSize(2048, 4096),
		),
	)

	opts := srv.Transport.Options()
	if opts.MaxRequestBodySize != 4242 {
		t.Errorf("MaxRequestBodySize = %d, want 4242", opts.MaxRequestBodySize)
	}
	if opts.ReadBufferSize != 2048 || opts.WriteBufferSize != 4096 {
		t.Errorf("buffer sizes = %d/%d, want 2048/4096", opts.ReadBufferSize, opts.WriteBufferSize)
	}
}

// TestTransportOptionsTakePrecedence confirms a raw transport option applied via
// WithTransportOptions overrides the value derived from a named server option.
func TestTransportOptionsTakePrecedence(t *testing.T) {
	srv := NewServer(
		WithHandlerTimeout(1*time.Second),
		WithTransportOptions(transport.WithHandlerTimeout(9*time.Second)),
	)

	if got := srv.Transport.Options().HandlerTimeout; got != 9*time.Second {
		t.Fatalf("HandlerTimeout = %v, want 9s (raw transport option should win)", got)
	}
}
