// Kunhua Huang 2026

package registry

import (
	"fmt"
	"time"
)

// ServiceInstance describes one reachable service endpoint in a registry.
type ServiceInstance struct {
	ID       string
	Service  string
	Version  string
	Address  string
	Port     int
	Metadata map[string]string
	Weight   int
	Status   InstanceStatus

	RegisterTime time.Time
	UpdateTime   time.Time
}

// InstanceStatus describes the lifecycle state of a registered service
// instance.
type InstanceStatus int

// ServiceInstance status values.
const (
	StatusUnknown InstanceStatus = iota
	StatusUp
	StatusDown
	StatusStarting
)

// String returns the symbolic status name.
func (s InstanceStatus) String() string {
	switch s {
	case StatusUp:
		return "UP"
	case StatusDown:
		return "DOWN"
	case StatusStarting:
		return "STARTING"
	default:
		return "UNKNOWN"
	}
}

// NewServiceInstance constructs a healthy service instance with generated ID,
// default weight, and initialized metadata.
func NewServiceInstance(service, address string, port int) *ServiceInstance {
	return &ServiceInstance{
		ID:           fmt.Sprintf("%s-%s:%d-%d", service, address, port, time.Now().Unix()),
		Service:      service,
		Address:      address,
		Port:         port,
		Metadata:     make(map[string]string),
		Weight:       100,
		Status:       StatusUp,
		RegisterTime: time.Now(),
		UpdateTime:   time.Now(),
	}
}

// Endpoint returns "host:port" for the instance.
func (si *ServiceInstance) Endpoint() string {
	return fmt.Sprintf("%s:%d", si.Address, si.Port)
}
