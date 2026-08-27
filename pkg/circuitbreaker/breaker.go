// Kunhua Huang 2026

package circuitbreaker

import (
	"RPCinGo/pkg/interceptor"
	"RPCinGo/pkg/protocol"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrCircuitOpen and ErrTooManyRequests are returned when the breaker rejects a
// call.
var (
	ErrCircuitOpen     = errors.New("circuit breaker is open")
	ErrTooManyRequests = errors.New("too many requests")
)

// Config tunes the circuit breaker's thresholds and timings.
type Config struct {
	MaxRequests      uint32
	MinRequests      uint32
	Interval         time.Duration
	Timeout          time.Duration
	FailureThreshold float64
	SuccessThreshold uint32
}

// DefaultConfig returns a conservative default circuit-breaker configuration.
func DefaultConfig() *Config {
	return &Config{
		MaxRequests:      1,
		MinRequests:      5,
		Interval:         10 * time.Second,
		Timeout:          60 * time.Second,
		FailureThreshold: 0.5,
		SuccessThreshold: 2,
	}
}

// CircuitBreaker guards a downstream call path using a sliding-window failure
// rate.
type CircuitBreaker struct {
	config *Config
	state  State
	window *SlidingWindow

	openTime          time.Time
	halfOpenSuccesses uint32
	halfOpenInFlight  uint32

	mu sync.RWMutex
}

// New returns a CircuitBreaker configured with config or DefaultConfig when
// config is nil.
func New(config *Config) *CircuitBreaker {
	if config == nil {
		config = DefaultConfig()
	}

	bucketTime := config.Interval / 10
	if bucketTime <= 0 {
		bucketTime = time.Second
	}

	return &CircuitBreaker{
		config: config,
		state:  StateClosed,
		window: NewSlidingWindow(10, bucketTime),
	}
}

// Call executes fn if the breaker allows it and records the outcome.
func (cb *CircuitBreaker) Call(ctx context.Context, fn func() (any, error)) (any, error) {
	if err := cb.beforeCall(); err != nil {
		return nil, err
	}

	result, err := fn()
	cb.afterCall(err)

	return result, err
}

// CallResponse is a typed helper around Call for RPC response functions.
func (cb *CircuitBreaker) CallResponse(ctx context.Context, fn func() (*protocol.Response, error)) (*protocol.Response, error) {
	v, err := cb.Call(ctx, func() (interface{}, error) {
		return fn()
	})
	if err != nil {
		return nil, err
	}
	resp, ok := v.(*protocol.Response)
	if !ok {
		return nil, fmt.Errorf("unexpected type %T", v)
	}
	return resp, nil
}

func (cb *CircuitBreaker) beforeCall() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return nil

	case StateOpen:
		if time.Since(cb.openTime) > cb.config.Timeout {
			cb.state = StateHalfOpen
			cb.halfOpenSuccesses = 0
			cb.halfOpenInFlight = 0
			return nil
		}

		return ErrCircuitOpen

	case StateHalfOpen:
		if cb.halfOpenInFlight >= cb.config.MaxRequests {
			return ErrTooManyRequests
		}

		cb.halfOpenInFlight++
		return nil

	default:
		return nil
	}
}

func (cb *CircuitBreaker) afterCall(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateHalfOpen && cb.halfOpenInFlight > 0 {
		cb.halfOpenInFlight--
	}

	if err != nil {
		cb.window.RecordFailure()
		cb.onFailure()
	} else {
		cb.window.RecordSuccess()
		cb.onSuccess()
	}
}

func (cb *CircuitBreaker) onFailure() {
	switch cb.state {
	case StateClosed:

		total := cb.window.Total()
		if total < int64(cb.config.MinRequests) {
			return
		}

		if cb.window.FailureRate() >= cb.config.FailureThreshold {
			cb.state = StateOpen
			cb.openTime = time.Now()
		}

	case StateHalfOpen:
		cb.state = StateOpen
		cb.openTime = time.Now()

	default:
	}
}

func (cb *CircuitBreaker) onSuccess() {
	switch cb.state {
	case StateHalfOpen:
		cb.halfOpenSuccesses++
		if cb.halfOpenSuccesses >= cb.config.SuccessThreshold {
			cb.state = StateClosed
			cb.window.Reset()
		}
	default:
	}
}

// State returns the current breaker state.
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Stats returns the current state and aggregated failure rate.
func (cb *CircuitBreaker) Stats() (state State, failureRate float64) {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return cb.state, cb.window.FailureRate()
}

// CircuitBreakerInterceptor returns an interceptor that applies circuit breaker logic to the server-side invocations.
func CircuitBreakerInterceptor(cb *CircuitBreaker) interceptor.Interceptor {
	return func(ctx context.Context, req *protocol.Request, invoker interceptor.Invoker) (any, error) {
		return cb.Call(ctx, func() (any, error) {
			return invoker(ctx, req)
		})
	}
}
