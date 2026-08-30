// Kunhua Huang 2026

package pool

import (
	"context"
	"fmt"
	"sync"

	"github.com/ecstasoy/RPCinGo/pkg/protocol"
)

// PoolManager lazily creates and caches one ConnectionPool per endpoint.
type PoolManager struct {
	pools map[string]*ConnectionPool
	mu    sync.RWMutex

	codecType    protocol.CodecType
	compressType protocol.CompressType

	maxPoolSize int
	minPoolSize int
}

// PoolManagerOption mutates a PoolManager before it starts creating pools.
type PoolManagerOption func(*PoolManager)

// WithManagerPoolSize sets the per-endpoint pool size the manager applies to
// every pool it creates. Without it the manager falls back to 100/10.
func WithManagerPoolSize(max, min int) PoolManagerOption {
	return func(pm *PoolManager) {
		if max > 0 {
			pm.maxPoolSize = max
		}
		if min >= 0 {
			pm.minPoolSize = min
		}
	}
}

// NewPoolManager returns a PoolManager that creates pools using codecType and
// compressType. Per-endpoint pool size defaults to 100/10 and can be overridden
// with WithManagerPoolSize.
func NewPoolManager(codecType protocol.CodecType, compressType protocol.CompressType, opts ...PoolManagerOption) *PoolManager {
	pm := &PoolManager{
		pools:        make(map[string]*ConnectionPool),
		codecType:    codecType,
		compressType: compressType,
		maxPoolSize:  100,
		minPoolSize:  10,
	}
	for _, o := range opts {
		o(pm)
	}
	return pm
}

// GetConnection returns a connection from the pool for addr, creating the pool
// on first use.
func (pm *PoolManager) GetConnection(ctx context.Context, addr string) (*PooledConnection, error) {
	// Double-checked locking
	// First check with read lock
	pm.mu.RLock()
	pool, exists := pm.pools[addr]
	pm.mu.RUnlock()

	// Fast path: pool exists
	if exists {
		return pool.GetWithContext(ctx)
	}

	// Acquire write lock to create the pool
	// Second check with write lock
	// prevents race condition when multiple goroutines try to create the same pool
	pm.mu.Lock()

	pool, exists = pm.pools[addr]
	if exists {
		pm.mu.Unlock()
		return pool.GetWithContext(ctx)
	}

	newPool, err := NewConnectionPool(
		addr,
		WithPoolSize(pm.maxPoolSize, pm.minPoolSize),
		WithPoolCodec(pm.codecType, pm.compressType),
	)
	if err != nil {
		pm.mu.Unlock()
		return nil, fmt.Errorf("new pool: %w", err)
	}

	pm.pools[addr] = newPool
	pm.mu.Unlock()

	return newPool.GetWithContext(ctx)
}

// RemovePool closes and removes the pool associated with addr.
func (pm *PoolManager) RemovePool(addr string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pool, exists := pm.pools[addr]
	if !exists {
		return nil
	}

	delete(pm.pools, addr)
	return pool.Close()
}

// Close closes and removes every managed pool.
func (pm *PoolManager) Close() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for addr, pool := range pm.pools {
		if err := pool.Close(); err != nil {
			return fmt.Errorf("close pool for %s: %w", addr, err)
		}
		delete(pm.pools, addr)
	}

	pm.pools = make(map[string]*ConnectionPool)
	return nil
}

// Stats returns point-in-time statistics for each managed pool.
func (pm *PoolManager) Stats() map[string]PoolStats {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	stats := make(map[string]PoolStats)
	for addr, pool := range pm.pools {
		stats[addr] = pool.Stats()
	}

	return stats
}
