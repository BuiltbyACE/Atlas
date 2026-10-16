package approvals

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNewSweeperClampsInvalidConfig(t *testing.T) {
	s := NewSweeper(nil, 0, 0, nil)
	if s.interval != time.Minute {
		t.Fatalf("interval = %v, want default %v", s.interval, time.Minute)
	}
	if s.limit != 100 {
		t.Fatalf("limit = %d, want default 100", s.limit)
	}
}

func TestSweeperRunStopsOnCancel(t *testing.T) {
	// A one-hour interval means the loop never sweeps, so a nil store is never
	// touched; the test is only about shutdown behaviour.
	sweeper := NewSweeper(NewService(nil, nil, nil, nil), time.Hour, 10, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- sweeper.Run(ctx) }()
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return on context cancellation")
	}
}

// TestSweeperClampsConfigAtConstruction guards the "bad environment cannot make
// the loop spin" promise for the values a positive interval would otherwise miss.
func TestNewSweeperKeepsValidConfig(t *testing.T) {
	s := NewSweeper(nil, 5*time.Second, 7, nil)
	if s.interval != 5*time.Second {
		t.Fatalf("interval = %v, want 5s", s.interval)
	}
	if s.limit != 7 {
		t.Fatalf("limit = %d, want 7", s.limit)
	}
	if s.logger == nil {
		t.Fatal("logger is nil, want a default logger")
	}
}
