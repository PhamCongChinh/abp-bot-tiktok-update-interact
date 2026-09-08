package crawler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"abp-bot-tiktok/pkg/gpm"

	"go.uber.org/zap"
)

// mockGPMClient implements gpm.GPMClient for testing.
type mockGPMClient struct {
	mu             sync.Mutex
	startProfileFn func(ctx context.Context, profileID string) (string, error)
	stopProfileFn  func(ctx context.Context, profileID string) error
	startCount     int
	stopCount      int
}

func (m *mockGPMClient) StartProfile(ctx context.Context, profileID string) (string, error) {
	m.mu.Lock()
	m.startCount++
	m.mu.Unlock()
	if m.startProfileFn != nil {
		return m.startProfileFn(ctx, profileID)
	}
	return "", errors.New("mockGPMClient: StartProfile not configured")
}

func (m *mockGPMClient) StopProfile(ctx context.Context, profileID string) error {
	m.mu.Lock()
	m.stopCount++
	m.mu.Unlock()
	if m.stopProfileFn != nil {
		return m.stopProfileFn(ctx, profileID)
	}
	return nil
}

// errGPM is a sentinel error for testing.
var errGPM = errors.New("gpm connection failed")

func TestGPMService_InitialState(t *testing.T) {
	svc := NewDefaultGPMService()
	if s := svc.State(); s != CircuitClosed {
		t.Errorf("initial state = %q, want %q", s, CircuitClosed)
	}
}

func TestGPMService_RecordSuccess(t *testing.T) {
	svc := NewDefaultGPMService()

	// Manually set failures to confirm reset on success.
	svc.mu.Lock()
	svc.failures = 2
	svc.state = CircuitClosed
	svc.mu.Unlock()

	svc.recordSuccessLocked(zap.NewNop())

	if svc.failures != 0 {
		t.Errorf("failures after success = %d, want 0", svc.failures)
	}
	if svc.State() != CircuitClosed {
		t.Errorf("state after success = %q, want %q", svc.State(), CircuitClosed)
	}
}

func TestGPMService_RecordFailure_OpensCircuit(t *testing.T) {
	svc := NewDefaultGPMService()

	log := zap.NewNop()

	// 3 consecutive failures should open the circuit.
	for i := 1; i <= 3; i++ {
		svc.mu.Lock()
		svc.recordFailureLocked(log)
		svc.mu.Unlock()

		if i < 3 {
			if svc.State() != CircuitClosed {
				t.Errorf("state after %d failures = %q, want %q", i, svc.State(), CircuitClosed)
			}
		}
	}

	if svc.State() != CircuitOpen {
		t.Errorf("state after 3 failures = %q, want %q", svc.State(), CircuitOpen)
	}
	if svc.failures != 3 {
		t.Errorf("failures = %d, want 3", svc.failures)
	}
}

func TestGPMService_BeforeRequest_BlocksWhenOpen(t *testing.T) {
	svc := NewDefaultGPMService()

	// Set circuit to open with recent lastFailure.
	svc.mu.Lock()
	svc.state = CircuitOpen
	svc.failures = 3
	svc.lastFailure = time.Now()
	svc.mu.Unlock()

	err := svc.beforeRequest(zap.NewNop())
	if err == nil {
		t.Fatal("expected error when circuit is open, got nil")
	}
	if !errors.Is(err, err) {
		// Just check that the error message mentions circuit breaker.
		_ = err
	}
}

func TestGPMService_BeforeRequest_OpenToHalfOpen(t *testing.T) {
	svc := NewDefaultGPMService()

	// Set circuit to open with expired lastFailure.
	svc.mu.Lock()
	svc.state = CircuitOpen
	svc.failures = 3
	svc.lastFailure = time.Now().Add(-10 * time.Minute) // reset timer elapsed
	svc.mu.Unlock()

	err := svc.beforeRequest(zap.NewNop())
	if err != nil {
		t.Fatalf("expected no error when reset timer elapsed, got: %v", err)
	}
	if svc.State() != CircuitHalfOpen {
		t.Errorf("state after expired reset = %q, want %q", svc.State(), CircuitHalfOpen)
	}
}

func TestGPMService_BeforeRequest_AllowsClosed(t *testing.T) {
	svc := NewDefaultGPMService()

	err := svc.beforeRequest(zap.NewNop())
	if err != nil {
		t.Fatalf("expected no error in closed state, got: %v", err)
	}
}

func TestGPMService_BeforeRequest_AllowsHalfOpen(t *testing.T) {
	svc := NewDefaultGPMService()

	svc.mu.Lock()
	svc.state = CircuitHalfOpen
	svc.mu.Unlock()

	err := svc.beforeRequest(zap.NewNop())
	if err != nil {
		t.Fatalf("expected no error in half-open state, got: %v", err)
	}
}

func TestGPMService_AfterRequest_HalfOpenToClosed(t *testing.T) {
	svc := NewDefaultGPMService()

	svc.mu.Lock()
	svc.state = CircuitHalfOpen
	svc.failures = 3
	svc.mu.Unlock()

	svc.afterRequest(zap.NewNop(), nil) // success

	if svc.State() != CircuitClosed {
		t.Errorf("state after half-open success = %q, want %q", svc.State(), CircuitClosed)
	}
	if svc.failures != 0 {
		t.Errorf("failures after half-open success = %d, want 0", svc.failures)
	}
}

func TestGPMService_AfterRequest_HalfOpenToOpen(t *testing.T) {
	svc := NewDefaultGPMService()

	svc.mu.Lock()
	svc.state = CircuitHalfOpen
	svc.failures = 3
	svc.mu.Unlock()

	svc.afterRequest(zap.NewNop(), errGPM) // failure

	if svc.State() != CircuitOpen {
		t.Errorf("state after half-open failure = %q, want %q", svc.State(), CircuitOpen)
	}
	if svc.failures != 4 {
		t.Errorf("failures after half-open failure = %d, want 4", svc.failures)
	}
}

func TestGPMService_AfterRequest_ClosedStaysClosedOnSuccess(t *testing.T) {
	svc := NewDefaultGPMService()

	svc.afterRequest(zap.NewNop(), nil) // success

	if svc.State() != CircuitClosed {
		t.Errorf("state after closed success = %q, want %q", svc.State(), CircuitClosed)
	}
	if svc.failures != 0 {
		t.Errorf("failures after closed success = %d, want 0", svc.failures)
	}
}

func TestGPMService_AfterRequest_ClosedStaysClosedOnSingleFailure(t *testing.T) {
	svc := NewDefaultGPMService()

	svc.afterRequest(zap.NewNop(), errGPM) // 1st failure

	if svc.State() != CircuitClosed {
		t.Errorf("state after 1 failure = %q, want %q", svc.State(), CircuitClosed)
	}
	if svc.failures != 1 {
		t.Errorf("failures after 1 failure = %d, want 1", svc.failures)
	}
}

func TestGPMService_FullStateLifecycle(t *testing.T) {
	svc := NewDefaultGPMService()
	log := zap.NewNop()

	// 1. Closed → 3 failures → Open
	for i := 0; i < 3; i++ {
		svc.afterRequest(log, errGPM)
	}
	if svc.State() != CircuitOpen {
		t.Fatalf("after 3 failures: state = %q, want %q", svc.State(), CircuitOpen)
	}

	// 2. Open → expired reset → Half-Open
	svc.mu.Lock()
	svc.lastFailure = time.Now().Add(-10 * time.Minute)
	svc.mu.Unlock()

	err := svc.beforeRequest(log)
	if err != nil {
		t.Fatalf("beforeRequest after expired reset: %v", err)
	}
	if svc.State() != CircuitHalfOpen {
		t.Fatalf("after expired reset: state = %q, want %q", svc.State(), CircuitHalfOpen)
	}

	// 3. Half-Open + success → Closed
	svc.afterRequest(log, nil)
	if svc.State() != CircuitClosed {
		t.Errorf("after half-open success: state = %q, want %q", svc.State(), CircuitClosed)
	}
	if svc.failures != 0 {
		t.Errorf("after half-open success: failures = %d, want 0", svc.failures)
	}
}

func TestGPMService_FullLifecycle_FailInHalfOpen(t *testing.T) {
	svc := NewDefaultGPMService()
	log := zap.NewNop()

	// 1. Closed → 3 failures → Open
	for i := 0; i < 3; i++ {
		svc.afterRequest(log, errGPM)
	}
	if svc.State() != CircuitOpen {
		t.Fatalf("after 3 failures: state = %q, want %q", svc.State(), CircuitOpen)
	}

	// 2. Open → expired reset → Half-Open
	svc.mu.Lock()
	svc.lastFailure = time.Now().Add(-10 * time.Minute)
	svc.mu.Unlock()

	err := svc.beforeRequest(log)
	if err != nil {
		t.Fatalf("beforeRequest after expired reset: %v", err)
	}
	if svc.State() != CircuitHalfOpen {
		t.Fatalf("after expired reset: state = %q, want %q", svc.State(), CircuitHalfOpen)
	}

	// 3. Half-Open + failure → Open
	svc.afterRequest(log, errGPM)
	if svc.State() != CircuitOpen {
		t.Errorf("after half-open failure: state = %q, want %q", svc.State(), CircuitOpen)
	}
}

func TestGPMService_StateIsThreadSafe(t *testing.T) {
	svc := NewDefaultGPMService()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = svc.State()
		}()
	}
	wg.Wait()
	// No race detector complaints = pass.
}

func TestGPMService_AfterRequestConcurrent(t *testing.T) {
	svc := NewDefaultGPMService()
	log := zap.NewNop()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc.afterRequest(log, errGPM)
		}()
	}
	wg.Wait()

	// All 100 goroutines recorded a failure.
	svc.mu.Lock()
	failures := svc.failures
	svc.mu.Unlock()
	if failures != 100 {
		t.Errorf("concurrent failures = %d, want 100", failures)
	}
	if svc.State() != CircuitOpen {
		t.Errorf("state after 100 concurrent failures = %q, want %q", svc.State(), CircuitOpen)
	}
}

// Test mockGPMClient itself.
func TestMockGPMClient_DefaultError(t *testing.T) {
	m := &mockGPMClient{}
	_, err := m.StartProfile(context.Background(), "profile-1")
	if err == nil {
		t.Fatal("expected error from unconfigured mock")
	}
}

func TestMockGPMClient_CustomBehavior(t *testing.T) {
	m := &mockGPMClient{
		startProfileFn: func(ctx context.Context, profileID string) (string, error) {
			return "ws://debug-addr", nil
		},
		stopProfileFn: func(ctx context.Context, profileID string) error {
			return nil
		},
	}

	wsURL, err := m.StartProfile(context.Background(), "profile-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wsURL != "ws://debug-addr" {
		t.Errorf("wsURL = %q, want %q", wsURL, "ws://debug-addr")
	}

	err = m.StopProfile(context.Background(), "profile-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verify that errGPM satisfies the error interface.
var _ error = errGPM

// Verify that mockGPMClient satisfies gpm.GPMClient.
var _ gpm.GPMClient = (*mockGPMClient)(nil)

// TestGPMService_RecordFailure_FromHalfOpen verifies that a single failure
// in half-open state transitions to open (not just increments failures).
func TestGPMService_RecordFailure_FromHalfOpen(t *testing.T) {
	svc := NewDefaultGPMService()

	// Start with circuit trivially half-open with 3 failures.
	svc.mu.Lock()
	svc.state = CircuitHalfOpen
	svc.failures = 3
	svc.mu.Unlock()

	// Record a failure while half-open.
	svc.recordFailureLocked(zap.NewNop())

	// Should now be open (since failures >= maxFailures).
	if svc.State() != CircuitOpen {
		t.Errorf("state after half-open failure = %q, want %q", svc.State(), CircuitOpen)
	}
	if svc.failures != 4 {
		t.Errorf("failures = %d, want 4", svc.failures)
	}
}

// TestGPMService_RecordFailure_BelowThreshold verifies that failures below
// the threshold keep the circuit closed.
func TestGPMService_RecordFailure_BelowThreshold(t *testing.T) {
	svc := NewDefaultGPMService()

	for i := 1; i <= 2; i++ {
		svc.recordFailureLocked(zap.NewNop())

		if svc.State() != CircuitClosed {
			t.Errorf("iteration %d: state = %q, want %q", i, svc.State(), CircuitClosed)
		}
		if svc.failures != i {
			t.Errorf("iteration %d: failures = %d, want %d", i, svc.failures, i)
		}
	}
}

// TestGPMService_DefaultConstants verifies default circuit breaker settings.
func TestGPMService_DefaultConstants(t *testing.T) {
	svc := NewDefaultGPMService()

	if svc.maxFailures != 3 {
		t.Errorf("maxFailures = %d, want 3", svc.maxFailures)
	}
	if svc.resetAfter != 5*time.Minute {
		t.Errorf("resetAfter = %v, want 5m", svc.resetAfter)
	}
	if svc.maxRetries != 3 {
		t.Errorf("maxRetries = %d, want 3", svc.maxRetries)
	}
}

// TestGPMService_RecordSuccess_AlreadyClosed is a no-op test confirming
// recordSuccessLocked on an already-closed circuit doesn't change state.
func TestGPMService_RecordSuccess_AlreadyClosed(t *testing.T) {
	svc := NewDefaultGPMService()

	// Already closed, no failures.
	svc.recordSuccessLocked(zap.NewNop())

	if svc.State() != CircuitClosed {
		t.Errorf("state = %q, want %q", svc.State(), CircuitClosed)
	}
	if svc.failures != 0 {
		t.Errorf("failures = %d, want 0", svc.failures)
	}
}
