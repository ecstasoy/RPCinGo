package circuitbreaker

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCircuitBreaker_Closed(t *testing.T) {
	cb := New(DefaultConfig())

	successFunc := func() (interface{}, error) {
		return "ok", nil
	}

	for i := 0; i < 10; i++ {
		_, err := cb.Call(context.Background(), successFunc)
		if err != nil {
			t.Errorf("call %d failed: %v", i, err)
		}
	}

	if cb.State() != StateClosed {
		t.Errorf("should be CLOSED, got %s", cb.State())
	}

	t.Log("✅ Closed state test passed")
}

func TestCircuitBreaker_Open(t *testing.T) {
	config := DefaultConfig()
	config.FailureThreshold = 0.5

	cb := New(config)

	failFunc := func() (interface{}, error) {
		return nil, errors.New("fail")
	}

	// Trigger 10 failures
	for i := 0; i < 10; i++ {
		cb.Call(context.Background(), failFunc)
	}

	state, failRate := cb.Stats()
	t.Logf("State: %s, FailureRate: %.2f", state, failRate)

	if state != StateOpen {
		t.Errorf("should be OPEN, got %s", state)
	}

	// Next call should fail fast
	_, err := cb.Call(context.Background(), failFunc)
	if err != ErrCircuitOpen {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}

	t.Log("✅ Open state test passed")
}

func TestCircuitBreaker_HalfOpen(t *testing.T) {
	config := DefaultConfig()
	config.Timeout = 100 * time.Millisecond
	config.SuccessThreshold = 2

	cb := New(config)

	// Trigger Open
	for i := 0; i < 10; i++ {
		cb.Call(context.Background(), func() (interface{}, error) {
			return nil, errors.New("fail")
		})
	}

	if cb.State() != StateOpen {
		t.Fatal("should be OPEN")
	}

	// Wait for timeout
	time.Sleep(150 * time.Millisecond)

	// Now should be Half-Open
	successFunc := func() (interface{}, error) {
		return "ok", nil
	}

	_, err := cb.Call(context.Background(), successFunc)
	if err != nil && err != ErrCircuitOpen {
		t.Logf("First call in Half-Open: %v", err)
	}

	if cb.State() != StateHalfOpen {
		t.Errorf("should be HALF_OPEN, got %s", cb.State())
	}

	// Success twice → Closed
	cb.Call(context.Background(), successFunc)
	cb.Call(context.Background(), successFunc)

	if cb.State() != StateClosed {
		t.Errorf("should be CLOSED after %d successes, got %s",
			config.SuccessThreshold, cb.State())
	}

	t.Log("✅ Half-Open state test passed")
}
