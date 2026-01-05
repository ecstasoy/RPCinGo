// Kunhua Huang 2026

package loadbalancer

import (
	"context"
	"crypto/md5"
	"fmt"
	"sort"
	"sync"
	"time"

	"RPCinGo/pkg/registry"
)

const defaultVirtualNodes = 150

type ConsistentHash struct {
	hashRing     []uint32
	nodes        map[uint32]*registry.ServiceInstance
	virtualNodes int

	instances []*registry.ServiceInstance
	mu        sync.RWMutex
}

func NewConsistentHash() BalancerWithOptions {
	return &ConsistentHash{
		nodes:        make(map[uint32]*registry.ServiceInstance),
		virtualNodes: defaultVirtualNodes,
	}
}

func (ch *ConsistentHash) Pick(ctx context.Context, instances []*registry.ServiceInstance) (*registry.ServiceInstance, error) {
	return ch.PickWithOptions(ctx, instances, nil)
}

func (ch *ConsistentHash) PickWithOptions(ctx context.Context, instances []*registry.ServiceInstance, opts *PickOptions) (*registry.ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, ErrNoInstances
	}

	ch.mu.Lock()
	if !ch.isSameInstances(instances) {
		ch.rebuild(instances)
	}
	ch.mu.Unlock()

	key := ""
	if opts != nil && opts.key != "" {
		key = opts.key
	} else {
		key = fmt.Sprintf("%d", time.Now().UnixNano())
	}

	ch.mu.RLock()
	defer ch.mu.RUnlock()

	hash := ch.hashKey(key)
	idx := sort.Search(len(ch.hashRing), func(i int) bool {
		return ch.hashRing[i] >= hash
	})

	if idx == len(ch.hashRing) {
		idx = 0
	}

	return ch.nodes[ch.hashRing[idx]], nil
}

func (ch *ConsistentHash) isSameInstances(instances []*registry.ServiceInstance) bool {
	if len(ch.instances) == 0 {
		return false
	}

	if len(instances) != len(ch.instances) {
		return false
	}

	cur := make(map[string]struct{}, len(instances))
	for _, inst := range instances {
		if inst == nil {
			return false
		}
		cur[inst.ID] = struct{}{}
	}

	for _, inst := range ch.instances {
		if inst == nil {
			return false
		}
		if _, ok := cur[inst.ID]; !ok {
			return false
		}
	}

	return true
}

func (ch *ConsistentHash) rebuild(instances []*registry.ServiceInstance) {
	ch.instances = instances
	ch.hashRing = ch.hashRing[:0]
	ch.nodes = make(map[uint32]*registry.ServiceInstance)

	for _, inst := range instances {
		for i := 0; i < ch.virtualNodes; i++ {
			key := fmt.Sprintf("%s-%d", inst.ID, i)
			hash := ch.hashKey(key)
			ch.hashRing = append(ch.hashRing, hash)
			ch.nodes[hash] = inst
		}
	}

	sort.Slice(ch.hashRing, func(i, j int) bool {
		return ch.hashRing[i] < ch.hashRing[j]
	})
}

func (ch *ConsistentHash) hashKey(key string) uint32 {
	hash := md5.Sum([]byte(key))
	return uint32(hash[0])<<24 | uint32(hash[1])<<16 | uint32(hash[2])<<8 | uint32(hash[3])
}

func (ch *ConsistentHash) Name() string {
	return "consistent-hash"
}
