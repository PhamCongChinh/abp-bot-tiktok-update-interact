package utils

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestRandInt(t *testing.T) {
	tests := []struct {
		name     string
		min, max int
	}{
		{"positive range", 1, 10},
		{"zero to positive", 0, 5},
		{"negative to positive", -5, 5},
		{"same value", 5, 5},
		{"min greater than max", 10, 1},
		{"large range", 0, 1000},
		{"single value", 7, 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := 0; i < 50; i++ {
				got := RandInt(tt.min, tt.max)
				expectedMin := tt.min
				expectedMax := tt.max
				if tt.min >= tt.max {
					// When min >= max, RandInt returns min
					if got != tt.min {
						t.Errorf("RandInt(%d, %d) = %d, want %d", tt.min, tt.max, got, tt.min)
					}
					return
				}
				if got < expectedMin || got > expectedMax {
					t.Errorf("RandInt(%d, %d) = %d, out of range [%d, %d]", tt.min, tt.max, got, expectedMin, expectedMax)
				}
			}
		})
	}
}

func TestSleep_ArgsSwap(t *testing.T) {
	// Sleep swaps min/max if min > max. We test that it doesn't panic.
	// Use very small values so it completes quickly.
	Sleep(5, 1) // min > max, should be swapped internally
	Sleep(1, 5) // normal
	Sleep(0, 0) // zero sleep
}

func TestSleepSeconds(t *testing.T) {
	// Just verify no panic with various inputs.
	SleepSeconds(0, 0)
	SleepSeconds(5, 1)
	SleepSeconds(1, 5)
}

func TestSleepContext_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	start := time.Now()
	SleepContext(ctx, 10*time.Second) // Would sleep 10s, but context is already cancelled.
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Errorf("SleepContext with cancelled context took %v, expected < 500ms", elapsed)
	}
}

func TestSleepContext_Completes(t *testing.T) {
	ctx := context.Background()

	start := time.Now()
	SleepContext(ctx, 50*time.Millisecond)
	elapsed := time.Since(start)

	if elapsed < 40*time.Millisecond {
		t.Errorf("SleepContext returned too early: %v, expected >= 40ms", elapsed)
	}
}

func TestSleepSecondsContext_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	SleepSecondsContext(ctx, 1, 5) // Would sleep 1-5s
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Errorf("SleepSecondsContext with cancelled context took %v, expected < 500ms", elapsed)
	}
}

func TestWaitForResources_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	log := zap.NewNop()
	_ = WaitForResources(ctx, log, "test-tag")
}

func TestWaitForResources_ContextCancelledDuringWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after a short delay to ensure we enter the wait loop.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	log := zap.NewNop()
	done := make(chan bool, 1)
	go func() {
		done <- WaitForResources(ctx, log, "test-tag")
	}()

	select {
	case <-done:
		// Returned without hanging — good.
	case <-time.After(5 * time.Second):
		t.Fatal("WaitForResources did not return within 5s on cancelled context")
	}
}
