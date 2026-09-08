package utils

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetryWithBackoff_Success(t *testing.T) {
	ctx := context.Background()
	attempts := 0
	err := RetryWithBackoff(ctx, 3, func() error {
		attempts++
		if attempts < 3 {
			return errors.New("transient failure")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetryWithBackoff_AllFail(t *testing.T) {
	ctx := context.Background()
	lastErr := errors.New("permanent failure")
	err := RetryWithBackoff(ctx, 2, func() error {
		return lastErr
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, lastErr) {
		t.Errorf("expected wrapped permanent failure, got %v", err)
	}
}

func TestRetryWithBackoff_FirstSuccess(t *testing.T) {
	ctx := context.Background()
	attempts := 0
	err := RetryWithBackoff(ctx, 3, func() error {
		attempts++
		return nil
	})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts)
	}
}

func TestRetryWithBackoff_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := RetryWithBackoff(ctx, 3, func() error {
		return errors.New("should not be reached")
	})
	if err == nil {
		t.Fatal("expected context cancelled error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestRetryWithBackoff_ContextTimeoutDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_ = RetryWithBackoff(ctx, 5, func() error {
		time.Sleep(100 * time.Millisecond) // exceed timeout
		return errors.New("slow operation")
	})
	// Either we get a deadline-exceeded error or the context error.
	// The important thing is that it doesn't hang.
}

func TestRetryWithBackoff_ZeroMaxAttempts(t *testing.T) {
	ctx := context.Background()
	called := false
	err := RetryWithBackoff(ctx, 0, func() error {
		called = true
		return errors.New("fail")
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !called {
		t.Error("expected fn to be called at least once")
	}
}

func TestRetryWithBackoff_NegativeMaxAttempts(t *testing.T) {
	ctx := context.Background()
	called := false
	err := RetryWithBackoff(ctx, -5, func() error {
		called = true
		return errors.New("fail")
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !called {
		t.Error("expected fn to be called at least once")
	}
}

func TestRetryWithBackoff_ContextCancelledDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	err := RetryWithBackoff(ctx, 3, func() error {
		cancel() // cancel after first attempt fails
		return errors.New("first fail")
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}
