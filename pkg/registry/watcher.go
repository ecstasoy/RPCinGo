// Kunhua Huang 2026

package registry

type EventType int

const (
	EventTypeAdd EventType = iota
	EventTypeUpdate
	EventTypeDelete
)

func (et EventType) String() string {
	switch et {
	case EventTypeAdd:
		return "ADD"
	case EventTypeUpdate:
		return "UPDATE"
	case EventTypeDelete:
		return "DELETE"
	default:
		return "UNKNOWN"
	}
}

type Event struct {
	Type     EventType
	Instance *ServiceInstance
}

type Watcher interface {
	Next() (*Event, error)
	Stop()
}
