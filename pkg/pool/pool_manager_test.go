package pool

import (
	"context"
	"testing"

	"RPCinGo/pkg/protocol"
)

func TestPoolManager_GetConnection(t *testing.T) {
	pm := NewPoolManager(protocol.CodecTypeJSON, protocol.CompressTypeNone)
	defer pm.Close()

	addr1 := "localhost:8080"
	addr2 := "localhost:8081"

	conn1, err := pm.GetConnection(context.Background(), addr1)
	if err != nil {
		t.Logf("Expected error (no server): %v", err)
	}
	if conn1 != nil {
		conn1.Release()
	}

	conn2, err := pm.GetConnection(context.Background(), addr2)
	if err != nil {
		t.Logf("Expected error (no server): %v", err)
	}
	if conn2 != nil {
		conn2.Release()
	}

	stats := pm.Stats()

	if len(stats) == 0 {
		t.Log("No pools created (expected if connection failed)")
	} else {
		t.Logf("Pools created: %d", len(stats))
		for addr, stat := range stats {
			t.Logf("  %s: %v", addr, stat)
		}
	}

	t.Log("✅ PoolManager test completed")
}

func TestPoolManager_MultipleAddresses(t *testing.T) {
	pm := NewPoolManager(protocol.CodecTypeJSON, protocol.CompressTypeNone)
	defer pm.Close()

	addresses := []string{
		"localhost:8080",
		"localhost:8081",
		"localhost:8082",
	}

	for _, addr := range addresses {
		_, err := pm.GetConnection(context.Background(), addr)
		if err != nil {
			t.Logf("Connection to %s failed (expected): %v", addr, err)
		}
	}

	stats := pm.Stats()
	t.Logf("Managed pools: %d", len(stats))

	t.Log("✅ Multiple addresses test completed")
}
