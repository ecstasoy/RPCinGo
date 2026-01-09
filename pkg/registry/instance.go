// Kunhua Huang 2026

package registry

import (
	"fmt"
	"time"
)

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

type InstanceStatus int

const (
	StatusUnknown InstanceStatus = iota
	StatusUp
	StatusDown
	StatusStarting
)

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

func (si *ServiceInstance) Endpoint() string {
	return fmt.Sprintf("%s:%d", si.Address, si.Port)
}
