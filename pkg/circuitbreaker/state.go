// Kunhua Huang 2026

package circuitbreaker

import "fmt"

// State describes the current state of a circuit breaker.
type State int

// Circuit breaker states.
const (
	StateClosed State = iota
	StateOpen
	StateHalfOpen
)

// String returns the symbolic state name.
func (s State) String() string {
	switch s {
	case StateClosed:
		return "CLOSED"
	case StateOpen:
		return "OPEN"
	case StateHalfOpen:
		return "HALF_OPEN"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", s)
	}
}
