package utils

import (
	"context"
	"fmt"
	"time"
)

// RetryWithBackoff executes fn up to maxAttempts times with exponential backoff.
// Returns the last error if all attempts fail, or the context error if ctx is
// cancelled during a backoff wait.
//
// Backoff sequence: 1s → 2s → 4s → 8s → ...
// The first attempt runs immediately (no backoff before it).
func RetryWithBackoff(ctx context.Context, maxAttempts int, fn func() error) error {
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Check context before each attempt.
		select {
		case <-ctx.Done():
			return fmt.Errorf("retry cancelled: %w", ctx.Err())
		default:
		}

		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		// Don't sleep after the last attempt.
		if attempt >= maxAttempts {
			break
		}

		// Exponential backoff: 1s, 2s, 4s, 8s, ...
		backoff := time.Duration(1<<(attempt-1)) * time.Second
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return fmt.Errorf("retry cancelled during backoff: %w", ctx.Err())
		}
	}

	return fmt.Errorf("failed after %d attempts: %w", maxAttempts, lastErr)
}
