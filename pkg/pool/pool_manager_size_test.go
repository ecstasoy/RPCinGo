package pool

import (
	"testing"

	"github.com/ecstasoy/RPCinGo/pkg/protocol"
)

// TestManagerPoolSizeHonored verifies that WithManagerPoolSize overrides the
// previously hardcoded 100/10 per-endpoint pool size, closing the gap where the
// discovery client's WithPoolSize was a silent no-op.
func TestManagerPoolSizeHonored(t *testing.T) {
	pm := NewPoolManager(protocol.CodecTypeJSON, protocol.CompressTypeNone,
		WithManagerPoolSize(500, 50))

	if pm.maxPoolSize != 500 {
		t.Errorf("maxPoolSize = %d, want 500", pm.maxPoolSize)
	}
	if pm.minPoolSize != 50 {
		t.Errorf("minPoolSize = %d, want 50", pm.minPoolSize)
	}
}

// TestManagerPoolSizeDefault verifies the default is preserved when no option is
// supplied.
func TestManagerPoolSizeDefault(t *testing.T) {
	pm := NewPoolManager(protocol.CodecTypeJSON, protocol.CompressTypeNone)
	if pm.maxPoolSize != 100 || pm.minPoolSize != 10 {
		t.Errorf("default pool size = %d/%d, want 100/10", pm.maxPoolSize, pm.minPoolSize)
	}
}
