// Kunhua Huang 2026

package registry

// EventType describes the kind of registry change emitted by a Watcher.
type EventType int

// Watch event types.
const (
	EventTypeAdd EventType = iota
	EventTypeUpdate
	EventTypeDelete
)

// String returns the symbolic event type name.
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

// Event reports a change to one service instance.
type Event struct {
	Type     EventType
	Instance *ServiceInstance
}

// Watcher delivers registry change events for one service.
type Watcher interface {
	Next() (*Event, error)
	Stop()
}
