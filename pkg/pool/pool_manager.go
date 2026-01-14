// Kunhua Huang 2026

package pool

import (
	"context"
	"fmt"
	"sync"

	"RPCinGo/pkg/protocol"
)

type PoolManager struct {
	pools map[string]*ConnectionPool
	mu    sync.RWMutex

	codecType    protocol.CodecType
	compressType protocol.CompressType

	maxPoolSize int
	minPoolSize int
}

func NewPoolManager(codecType protocol.CodecType, compressType protocol.CompressType) *PoolManager {
	return &PoolManager{
		pools:        make(map[string]*ConnectionPool),
		codecType:    codecType,
		compressType: compressType,
		maxPoolSize:  100,
		minPoolSize:  10,
	}
}

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

func (pm *PoolManager) Stats() map[string]PoolStats {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	stats := make(map[string]PoolStats)
	for addr, pool := range pm.pools {
		stats[addr] = pool.Stats()
	}

	return stats
}
