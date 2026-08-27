# Registry Layer Documentation

## Overview

### What is Registry Layer?

The Registry layer provides **service registration and discovery** capabilities, enabling dynamic service management in distributed systems.

### Responsibilities

```
✅ Service registration (server-side)
✅ Service discovery (client-side)
✅ Real-time change notification (Watch)
✅ Health checking
✅ Automatic cleanup (Lease mechanism)
```

### Position in Architecture

```
Application Layer
    ↓
RPC Client/Server
    ↓
Registry Layer     ← This layer
    ↓
etcd/Consul/Memory
```

---

## Core Components

### 1. ServiceInstance

**File**: `pkg/registry/instance.go`

**Purpose**: Represents a service instance

```go
type ServiceInstance struct {
    ID       string            // Unique identifier
    Service  string            // Service name
    Version  string            // Service version
    Address  string            // IP address
    Port     int               // Port number
    Metadata map[string]string // Additional info
    Weight   int               // Load balancing weight
    Status   InstanceStatus    // UP/DOWN/STARTING
    
    RegisterTime time.Time
    UpdateTime   time.Time
}
```

**Usage**:

```go
instance := registry.NewServiceInstance("UserService", "192.168.1.10", 8080)
instance.Metadata["version"] = "v1.0.0"
instance.Metadata["region"] = "us-west"
instance.Weight = 100

endpoint := instance.Endpoint()  // "192.168.1.10:8080"
```

---

### 2. Registry Interface

**File**: `pkg/registry/registry.go`

**Purpose**: Service registration interface (server-side)

```go
type Registry interface {
    Register(ctx context.Context, instance *ServiceInstance) error
    Deregister(ctx context.Context, service, instanceID string) error
    Update(ctx context.Context, instance *ServiceInstance) error
    Heartbeat(ctx context.Context, service, instanceID string) error
    Close() error
}
```

**Workflow**:

```
Server Startup:
1. Create ServiceInstance
2. Register to Registry
3. Start heartbeat goroutine
4. Start serving

Server Shutdown:
1. Deregister from Registry
2. Stop serving
```

---

### 3. Discovery Interface

**File**: `pkg/registry/registry.go`

**Purpose**: Service discovery interface (client-side)

```go
type Discovery interface {
    GetInstances(ctx context.Context, service string) ([]*ServiceInstance, error)
    Watch(ctx context.Context, service string) (Watcher, error)
    Close() error
}
```

**Usage**:

```go
instances, err := discovery.GetInstances(ctx, "UserService")
// Returns: [
//   {Address: "192.168.1.10", Port: 8080, Status: UP},
//   {Address: "192.168.1.11", Port: 8080, Status: UP},
// ]

for _, inst := range instances {
    if inst.Status == registry.StatusUp {
        // Use this instance
        endpoint := inst.Endpoint()
    }
}
```

---

### 4. Watcher Interface

**File**: `pkg/registry/watcher.go`

**Purpose**: Real-time service change notification

```go
type Watcher interface {
    Next() (*Event, error)  // Block until next event
    Stop()                  // Stop watching
}

type Event struct {
    Type     EventType        // ADD/UPDATE/DELETE
    Instance *ServiceInstance
}
```

**Usage**:

```go
watcher, err := discovery.Watch(ctx, "UserService")
defer watcher.Stop()

go func() {
    for {
        event, err := watcher.Next()  // Blocks here
        if err != nil {
            break
        }
        
        switch event.Type {
        case registry.EventTypeAdd:
            // New instance online
            addToLoadBalancer(event.Instance)
        case registry.EventTypeDelete:
            // Instance offline
            removeFromLoadBalancer(event.Instance)
        case registry.EventTypeUpdate:
            // Instance updated
            updateLoadBalancer(event.Instance)
        }
    }
}()
```

---

## Implementations

### 1. Memory Registry

**File**: `pkg/registry/memory/memory.go`

**Purpose**: In-memory implementation for testing

**Features**:
```
✅ No external dependencies
✅ Fast (pure memory)
✅ Thread-safe (sync.RWMutex)
✅ Watch support
```

**Use Cases**:
- Unit testing
- Local development
- Integration testing
- Demos

**Usage**:

```go
reg := memory.NewRegistry()
defer reg.Close()

instance := registry.NewServiceInstance("UserService", "localhost", 8080)
reg.Register(ctx, instance)

instances, _ := reg.GetInstances(ctx, "UserService")
```

**Implementation Details**:

```go
type Registry struct {
    instances map[string]*ServiceInstance  // Storage
    watchers  map[string][]chan *Event     // Watch subscriptions
    mu        sync.RWMutex                 // Concurrency control
}

Key design:
- instances: ID → Instance mapping
- watchers: Service → []EventChannel mapping
- notify(): Broadcast events to watchers
```

---

### 2. etcd Registry

**Files**: 
- `pkg/registry/etcd/etcd.go` (client wrapper)
- `pkg/registry/etcd/registry.go` (registration)
- `pkg/registry/etcd/discovery.go` (discovery)
- `pkg/registry/etcd/watcher.go` (watch)

**Purpose**: Production-ready implementation with etcd

**Features**:
```
✅ Distributed (cluster support)
✅ Persistent (survives restarts)
✅ Lease mechanism (auto-cleanup)
✅ Watch mechanism (real-time)
✅ Strong consistency (Raft)
```

**Key Design: Lease Mechanism**

```
Problem:
  Service crashes → Cannot deregister → Stale data

Solution:
  1. Create Lease with TTL (10s)
  2. Register with Lease
  3. Keep-alive every 3s
  4. If crash → No keep-alive → Lease expires → Auto-delete
```

**Workflow**:

```go
// 1. Create Registry
config := etcd.DefaultConfig()
reg, err := etcd.NewEtcdRegistry(config)

// 2. Register (with Lease)
instance := registry.NewServiceInstance("UserService", "192.168.1.10", 8080)
reg.Register(ctx, instance)
// Internal: Creates Lease, starts keep-alive goroutine

// 3. Query
disc, _ := etcd.NewEtcdDiscovery(config)
instances, _ := disc.GetInstances(ctx, "UserService")

// 4. Watch
watcher, _ := disc.Watch(ctx, "UserService")
event, _ := watcher.Next()  // Blocks until change

// 5. Cleanup
reg.Deregister(ctx, "UserService", instance.ID)
reg.Close()  // Revokes Lease
```

---

## Design Principles

### 1. Key Structure in etcd

```
Pattern: /{prefix}/{service}/{instanceID}

Examples:
/rpc/services/UserService/instance-192.168.1.10:8080-1735804800
/rpc/services/UserService/instance-192.168.1.11:8080-1735804801
/rpc/services/OrderService/instance-192.168.1.20:8080-1735804802

Benefits:
✅ Hierarchical organization
✅ Easy prefix query
✅ Service isolation
```

### 2. Lease Strategy

```
TTL: 10 seconds
Keep-Alive Interval: 3 seconds (TTL/3)

Timeline:
t=0s:  Create Lease, Register
t=3s:  Keep-alive (TTL → 10s)
t=6s:  Keep-alive (TTL → 10s)
t=9s:  Keep-alive (TTL → 10s)
...

If service crashes at t=5s:
t=5s:  Service dies
t=6s:  No keep-alive
t=9s:  No keep-alive
t=15s: Lease expires → Key deleted → Watch notified
```

### 3. Watch Pattern

```
Event Flow:
1. Client creates Watch
2. etcd sends existing data (optional)
3. Client receives events in real-time
4. Client updates local cache

Benefits:
✅ Real-time (milliseconds)
✅ No polling (efficient)
✅ Reliable (TCP connection)
```

---

## Usage Guide

### Server-Side Registration

```go
package main

import (
    "context"
    "github.com/ecstasoy/RPCinGo/pkg/registry"
    "github.com/ecstasoy/RPCinGo/pkg/registry/etcd"
)

func main() {
    config := etcd.DefaultConfig()
    config.Endpoints = []string{"localhost:2379"}
    
    reg, err := etcd.NewEtcdRegistry(config)
    if err != nil {
        panic(err)
    }
    defer reg.Close()
    
    instance := registry.NewServiceInstance(
        "UserService",
        "192.168.1.10",
        8080,
    )
    
    if err := reg.Register(context.Background(), instance); err != nil {
        panic(err)
    }
    
    // Keep-alive is automatic
    
    // On shutdown
    defer reg.Deregister(context.Background(), "UserService", instance.ID)
}
```

---

### Client-Side Discovery

```go
package main

import (
    "context"
    "github.com/ecstasoy/RPCinGo/pkg/registry/etcd"
)

func main() {
    config := etcd.DefaultConfig()
    disc, err := etcd.NewEtcdDiscovery(config)
    if err != nil {
        panic(err)
    }
    defer disc.Close()
    
    // Query instances
    instances, err := disc.GetInstances(context.Background(), "UserService")
    if err != nil {
        panic(err)
    }
    
    for _, inst := range instances {
        fmt.Printf("Found: %s\n", inst.Endpoint())
    }
    
    // Watch for changes
    watcher, _ := disc.Watch(context.Background(), "UserService")
    defer watcher.Stop()
    
    go func() {
        for {
            event, err := watcher.Next()
            if err != nil {
                return
            }
            
            fmt.Printf("Event: %s - %s\n", event.Type, event.Instance.Endpoint())
        }
    }()
}
```

---

## Performance

### Memory Registry

```
Register:     < 1μs (map insert)
GetInstances: O(n) iteration
Watch:        Channel-based (fast)
Concurrency:  RWMutex protected
```

### etcd Registry

```
Register:     1-5ms (network + etcd write)
GetInstances: 1-3ms (network + etcd read)
Watch:        Real-time push (< 10ms)
Keep-Alive:   Every 3s (background)
```

---

## Comparison

### Memory vs etcd

| Feature | Memory | etcd |
|---------|--------|------|
| **Persistence** | ❌ No | ✅ Yes |
| **Distributed** | ❌ No | ✅ Yes |
| **Auto-cleanup** | ❌ No | ✅ Lease |
| **Watch** | ✅ Yes | ✅ Yes |
| **Performance** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ |
| **Use Case** | Testing | Production |

---

## Best Practices

### 1. Always Use Lease (etcd)

```go
// ✅ Good: With Lease
reg.Register(ctx, instance)  // Automatic Lease handling

// ❌ Bad: Manual cleanup
etcd.Put(key, value)  // No auto-cleanup if crash
```

### 2. Handle Watch Errors

```go
watcher, _ := disc.Watch(ctx, service)

for {
    event, err := watcher.Next()
    if err != nil {
        // Reconnect logic
        watcher.Stop()
        watcher, _ = disc.Watch(ctx, service)
        continue
    }
    
    handleEvent(event)
}
```

### 3. Cache Instances

```go
type ClientCache struct {
    instances []*ServiceInstance
    mu        sync.RWMutex
}

// Update cache from Watch
watcher, _ := disc.Watch(ctx, service)
for {
    event, _ := watcher.Next()
    
    cache.mu.Lock()
    switch event.Type {
    case EventTypeAdd:
        cache.instances = append(cache.instances, event.Instance)
    case EventTypeDelete:
        cache.remove(event.Instance.ID)
    }
    cache.mu.Unlock()
}

// Use cached data (fast!)
cache.mu.RLock()
instances := cache.instances
cache.mu.RUnlock()
```

---

## Testing

### Test Coverage

```
Test Cases: 4
Files:
  - memory_test.go: 2 tests
  - etcd_test.go: 2 tests

Coverage:
  - Memory: ~80%
  - etcd: Requires etcd running
```

### Running Tests

```bash
# Memory tests (no dependencies)
go test ./pkg/registry/memory -v

# etcd tests (requires etcd)
etcd &  # Start etcd
go test ./pkg/registry/etcd -v
```

---

## Integration

### With RPC Server

```go
type Server struct {
    registry registry.Registry
}

func (s *Server) Start() {
    // 1. Register to registry
    instance := registry.NewServiceInstance(
        s.serviceName,
        s.address,
        s.port,
    )
    
    s.registry.Register(ctx, instance)
    
    // 2. Start serving
    s.serve()
    
    // 3. Deregister on shutdown
    defer s.registry.Deregister(ctx, s.serviceName, instance.ID)
}
```

### With RPC Client (Future)

```go
type Client struct {
    discovery registry.Discovery
}

func (c *Client) Call(service, method string, args interface{}) {
    // 1. Discover instances
    instances, _ := c.discovery.GetInstances(ctx, service)
    
    // 2. Load balance (next section)
    instance := c.loadBalancer.Pick(instances)
    
    // 3. Call
    conn := c.getConnection(instance.Endpoint())
    return conn.Call(...)
}
```

---

## etcd Operations

### Viewing Data

```bash
# List all services
etcdctl get "/rpc/services/" --prefix

# List specific service
etcdctl get "/rpc/services/UserService/" --prefix --keys-only

# Pretty print
etcdctl get "/rpc/services/UserService/" --prefix --print-value-only | jq
```

### Monitoring

```bash
# Watch changes
etcdctl watch "/rpc/services/UserService/" --prefix

# Check leases
etcdctl lease list

# Lease details
etcdctl lease timetolive <lease-id> --keys
```

---

## Troubleshooting

### Common Issues

#### 1. Connection refused

```
Error: dial tcp 127.0.0.1:2379: connect: connection refused

Solution:
- Check if etcd is running: ps aux | grep etcd
- Start etcd: etcd
- Check endpoint: netstat -an | grep 2379
```

#### 2. Lease expired too quickly

```
Problem: Instances disappear after 10s

Cause: Keep-alive not working

Check:
- listenKeepAlive() goroutine running?
- Network connectivity
- etcd server healthy
```

#### 3. Watch not receiving events

```
Problem: Watch created but no events

Check:
- Correct prefix (/rpc/services/Service/)
- Watch created before events happen
- Channel not blocked
```

---

## Future Enhancements

### Planned Features

```
🔜 Health checking
   - Periodic health check of instances
   - Auto mark DOWN if unhealthy

🔜 Metadata filtering
   - Query by version: GetInstances(service, version="v1.0")
   - Query by region: GetInstances(service, region="us-west")

🔜 Consul support
   - Alternative to etcd
   - Built-in health checking

🔜 Nacos support
   - Popular in China
   - Rich features
```

---

## Performance Characteristics

### Memory Registry

```
Operation         Time        Notes
──────────────────────────────────────
Register          < 1μs       Map insert
GetInstances      O(n)        Full scan
Watch             Instant     Channel-based
Deregister        < 1μs       Map delete
```

### etcd Registry

```
Operation         Time        Notes
──────────────────────────────────────
Register          1-5ms       Network + Write
GetInstances      1-3ms       Network + Read
Watch             < 10ms      Push notification
Keep-Alive        Background  Every 3s
```

---

## Design Patterns

```
✅ Interface Abstraction
   - Registry/Discovery interfaces
   - Multiple implementations

✅ Observer Pattern
   - Watcher observes Registry changes
   - Event-driven updates

✅ Strategy Pattern
   - Pluggable Registry backends
   - Memory/etcd/Consul

✅ Lease Pattern
   - Automatic resource cleanup
   - Fault tolerance
```

---

## Dependencies

```
Registry Layer:
  Depends on:
    - Protocol layer (for service info)
    - go.etcd.io/etcd/client/v3 (etcd only)
  
  Used by:
    - RPC Client (service discovery)
    - RPC Server (service registration)
```

---

## Example: Complete Flow

### Server Registration

```go
// 1. Create registry
reg, _ := etcd.NewEtcdRegistry(&etcd.Config{
    Endpoints: []string{"localhost:2379"},
    LeaseTTL:  10,
})
defer reg.Close()

// 2. Create instance
instance := registry.NewServiceInstance("UserService", "192.168.1.10", 8080)
instance.Metadata["version"] = "v1.0.0"

// 3. Register
reg.Register(ctx, instance)
// etcd stores:
//   Key: /rpc/services/UserService/instance-xxx
//   Value: {"address":"192.168.1.10:8080",...}
//   Lease: 10s TTL

// 4. Server runs...
// Keep-alive happens automatically

// 5. Shutdown
reg.Deregister(ctx, "UserService", instance.ID)
```

### Client Discovery

```go
// 1. Create discovery
disc, _ := etcd.NewEtcdDiscovery(&etcd.Config{
    Endpoints: []string{"localhost:2379"},
})
defer disc.Close()

// 2. Get instances
instances, _ := disc.GetInstances(ctx, "UserService")
// Returns: All UP instances

// 3. Pick one (load balancing - next section)
instance := instances[0]

// 4. Connect and call
client := rpc.NewClient(instance.Endpoint())
result, _ := client.Call(...)
```

---

## Comparison with Java Version

| Feature | Java (Zookeeper) | Go (etcd) |
|---------|------------------|-----------|
| **Backend** | Zookeeper | etcd |
| **Consensus** | ZAB | Raft |
| **Watch** | ✅ Yes | ✅ Yes |
| **Lease** | Ephemeral Node | Lease |
| **Performance** | Good | Better |
| **Ease of Use** | Complex | Simpler |

---

## Next Steps

### Integration Needed

```
1. RPC Server integration
   - Auto-register on start
   - Auto-deregister on stop

2. RPC Client integration
   - Auto-discover instances
   - Cache + Watch updates
   - Pool manager (one pool per address)

3. Load Balancer
   - Pick instance from list
   - Multiple algorithms
```

---

**Document Version**: v1.0  
**Last Updated**: 2026-01-03  
**Author**: Kunhua Huang




