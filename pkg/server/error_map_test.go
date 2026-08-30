package server

import (
	"errors"
	"testing"

	"github.com/ecstasoy/RPCinGo/pkg/protocol"
)

// TestErrorTableRoundTrip verifies that every sentinel in errorTable encodes to
// its code and decodes back to a value matching the sentinel. This is the
// invariant the single table exists to guarantee: the two directions cannot
// drift apart.
func TestErrorTableRoundTrip(t *testing.T) {
	for _, m := range errorTable {
		code, msg := mapError(m.sentinel, "Svc", "Method")
		if code != m.code {
			t.Errorf("mapError(%v) code = %d, want %d", m.sentinel, code, m.code)
		}
		if msg == "" {
			t.Errorf("mapError(%v) produced empty message", m.sentinel)
		}

		resp := protocol.NewErrorResponse(1, protocol.NewError(code, msg))
		got := unmapError(resp)
		if !errors.Is(got, m.sentinel) {
			t.Errorf("unmapError(code=%d) = %v, want errors.Is(_, %v)", code, got, m.sentinel)
		}
	}
}

// TestMapErrorUnknownIsInternal confirms unrecognized errors map to Internal.
func TestMapErrorUnknownIsInternal(t *testing.T) {
	code, msg := mapError(errors.New("boom"), "Svc", "Method")
	if code != protocol.ErrorCodeInternal {
		t.Errorf("unknown error code = %d, want %d (Internal)", code, protocol.ErrorCodeInternal)
	}
	if msg == "" {
		t.Error("expected non-empty message for internal error")
	}
}

// TestUnmapErrorSuccessIsNil confirms a success response decodes to nil.
func TestUnmapErrorSuccessIsNil(t *testing.T) {
	resp := protocol.NewSuccessResponse(1, nil)
	if err := unmapError(resp); err != nil {
		t.Errorf("unmapError(success) = %v, want nil", err)
	}
}

// TestUnmapErrorUnknownCode confirms codes without a sentinel degrade to a
// formatted error rather than panicking or returning nil.
func TestUnmapErrorUnknownCode(t *testing.T) {
	resp := protocol.NewErrorResponse(1, protocol.NewError(protocol.ErrorCodePermissionDenied, "denied"))
	err := unmapError(resp)
	if err == nil {
		t.Fatal("expected non-nil error for unmapped code")
	}
	if sentinelForCode(protocol.ErrorCodePermissionDenied) != nil {
		t.Error("PermissionDenied unexpectedly has a sentinel")
	}
}
