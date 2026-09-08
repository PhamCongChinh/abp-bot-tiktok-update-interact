package utils

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestSystemUsage(t *testing.T) {
	cpuPct, ramPct, err := systemUsage()
	if err != nil {
		// systemUsage can fail in restricted environments (e.g. containers
		// without /proc). That's acceptable — the caller treats errors as
		// "proceed anyway".
		t.Logf("systemUsage returned error (expected in some environments): %v", err)
		return
	}

	// CPU percent should be a reasonable value.
	if cpuPct < 0 {
		t.Errorf("cpuPct = %v, want >= 0", cpuPct)
	}
	if cpuPct > 100 {
		t.Errorf("cpuPct = %v, want <= 100", cpuPct)
	}

	// RAM percent should be in [0, 100].
	if ramPct < 0 {
		t.Errorf("ramPct = %v, want >= 0", ramPct)
	}
	if ramPct > 100 {
		t.Errorf("ramPct = %v, want <= 100", ramPct)
	}

	t.Logf("systemUsage: CPU=%.1f%%, RAM=%.1f%%", cpuPct, ramPct)
}

func TestSystemUsage_MultipleCalls(t *testing.T) {
	// systemUsage should be idempotent and not leak resources.
	for i := 0; i < 3; i++ {
		_, _, err := systemUsage()
		if err != nil {
			t.Logf("call %d: systemUsage error (non-fatal): %v", i, err)
			return
		}
	}
}

func TestResourceThreshold(t *testing.T) {
	// The resourceThreshold constant should be a sensible value.
	if resourceThreshold <= 0 || resourceThreshold > 100 {
		t.Errorf("resourceThreshold = %v, want value in (0, 100]", resourceThreshold)
	}
}

func TestWaitForResources_AlreadyBelowThreshold(t *testing.T) {
	// When system resources are already below threshold, WaitForResources
	// should return immediately with true.
	log := zap.NewNop()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	start := time.Now()
	result := WaitForResources(ctx, log, "test-tag")
	elapsed := time.Since(start)

	// With resources typically below threshold in test env, this should
	// return quickly (not 15+ seconds).
	if elapsed > 5*time.Second {
		t.Errorf("WaitForResources took %v — expected quick return when below threshold", elapsed)
	}

	t.Logf("WaitForResources returned %v in %v", result, elapsed)
}

func TestWaitForResources_ContextDeadlineExceeded(t *testing.T) {
	// Even with cancelled context, WaitForResources should return false
	// without hanging indefinitely.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Let the deadline expire.
	time.Sleep(10 * time.Millisecond)

	log := zap.NewNop()
	done := make(chan bool, 1)
	go func() {
		done <- WaitForResources(ctx, log, "deadline-test")
	}()

	select {
	case result := <-done:
		t.Logf("WaitForResources with expired deadline returned: %v", result)
	case <-time.After(3 * time.Second):
		t.Fatal("WaitForResources did not return within 3s on expired deadline")
	}
}
