package crawler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"abp-bot-tiktok/pkg/gpm"

	"github.com/playwright-community/playwright-go"
	"go.uber.org/zap"
)

// CircuitState represents the three states of the circuit breaker pattern.
type CircuitState string

const (
	// CircuitClosed is the normal state — requests are allowed.
	CircuitClosed CircuitState = "closed"
	// CircuitOpen is the tripped state — requests are blocked.
	CircuitOpen CircuitState = "open"
	// CircuitHalfOpen is the probe state — a single test request is allowed
	// to determine if the downstream service has recovered.
	CircuitHalfOpen CircuitState = "half-open"
)

const (
	defaultMaxFailures = 3
	defaultResetAfter  = 5 * time.Minute
	defaultMaxRetries  = 3
)

// GPMService manages GPM profile connections with a 3-state circuit breaker.
//
// States:
//   - closed:   normal operation, requests proceed.
//   - open:     after maxFailures consecutive failures, requests are blocked
//     for resetAfter before transitioning to half-open.
//   - half-open: a single probe request is allowed; success → closed,
//     failure → open.
type GPMService struct {
	maxFailures int
	resetAfter  time.Duration
	maxRetries  int

	mu          sync.Mutex
	failures    int
	lastFailure time.Time
	state       CircuitState
}

// NewGPMService creates a GPMService with default circuit breaker settings.
func NewGPMService() *GPMService {
	return &GPMService{
		maxFailures: defaultMaxFailures,
		resetAfter:  defaultResetAfter,
		maxRetries:  defaultMaxRetries,
		state:       CircuitClosed,
	}
}

// NewDefaultGPMService creates a GPMService with default circuit breaker
// settings.
func NewDefaultGPMService() *GPMService {
	return NewGPMService()
}

// State returns the current circuit breaker state. Thread-safe.
func (s *GPMService) State() CircuitState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// Connect attempts to establish a GPM browser connection through the circuit
// breaker. If the circuit is open, it returns an error immediately without
// calling GPM. On success, the circuit transitions to closed; on failure,
// failures are counted and the circuit may open.
func (s *GPMService) Connect(ctx context.Context, pw *playwright.Playwright, gpmClient gpm.GPMClient, profileID string, log *zap.Logger) (playwright.Browser, playwright.BrowserContext, error) {
	if err := s.beforeRequest(log); err != nil {
		return nil, nil, err
	}

	browser, bctx, err := s.connectGPMWithRetry(ctx, pw, gpmClient, profileID, log)
	s.afterRequest(log, err)
	return browser, bctx, err
}

// beforeRequest checks whether the circuit allows a request. It may transition
// open → half-open if the reset timer has elapsed. Returns an error when the
// circuit is open.
func (s *GPMService) beforeRequest(log *zap.Logger) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch s.state {
	case CircuitClosed:
		return nil
	case CircuitHalfOpen:
		return nil
	case CircuitOpen:
		if time.Since(s.lastFailure) >= s.resetAfter {
			s.state = CircuitHalfOpen
			log.Sugar().Infof("GPM circuit breaker: open -> half-open (reset timer elapsed)")
			return nil
		}
		remaining := s.resetAfter - time.Since(s.lastFailure)
		return fmt.Errorf("gpm circuit breaker open: %d consecutive failures, reset in %v", s.failures, remaining.Round(time.Second))
	default:
		return nil
	}
}

// afterRequest records the outcome of a GPM connection attempt and transitions
// the circuit state accordingly.
func (s *GPMService) afterRequest(log *zap.Logger, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err != nil {
		s.recordFailureLocked(log)
	} else {
		s.recordSuccessLocked(log)
	}
}

// recordFailureLocked increments the failure counter and transitions to open
// when the threshold is reached. Caller must hold s.mu.
func (s *GPMService) recordFailureLocked(log *zap.Logger) {
	s.failures++
	s.lastFailure = time.Now()

	if s.failures >= s.maxFailures && s.state != CircuitOpen {
		oldState := s.state
		s.state = CircuitOpen
		log.Sugar().Warnf("GPM circuit breaker: %s -> open (failures: %d/%d)", oldState, s.failures, s.maxFailures)
	}
}

// recordSuccessLocked resets failures and transitions to closed. Caller must
// hold s.mu.
func (s *GPMService) recordSuccessLocked(log *zap.Logger) {
	oldState := s.state
	s.failures = 0

	if oldState != CircuitClosed {
		s.state = CircuitClosed
		log.Sugar().Infof("GPM circuit breaker: %s -> closed", oldState)
	}
}

// connectGPMWithRetry attempts to connect to GPM with exponential backoff retry.
func (s *GPMService) connectGPMWithRetry(ctx context.Context, pw *playwright.Playwright, gpmClient gpm.GPMClient, profileID string, log *zap.Logger) (playwright.Browser, playwright.BrowserContext, error) {
	var browser playwright.Browser
	var browserCtx playwright.BrowserContext
	var err error

	for attempt := 1; attempt <= s.maxRetries; attempt++ {
		browser, browserCtx, err = s.connectGPM(ctx, pw, gpmClient, profileID, log)
		if err == nil {
			return browser, browserCtx, nil
		}
		if browser != nil {
			_ = browser.Close()
		}

		// Stop the profile on failure — if context is cancelled, return immediately.
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		default:
		}
		_ = gpmClient.StopProfile(ctx, profileID)

		if attempt < s.maxRetries {
			backoff := backoffDuration(attempt)
			log.Sugar().Warnf("GPM reconnect attempt %d/%d, retrying in %v: %v", attempt, s.maxRetries, backoff, err)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			}
		}
	}
	return nil, nil, fmt.Errorf("connectGPMWithRetry: failed after %d attempts: %w", s.maxRetries, err)
}

// connectGPM starts a GPM profile and connects via CDP.
func (s *GPMService) connectGPM(ctx context.Context, pw *playwright.Playwright, gpmClient gpm.GPMClient, profileID string, log *zap.Logger) (playwright.Browser, playwright.BrowserContext, error) {
	_ = log // reserved for future use
	wsURL, err := gpmClient.StartProfile(ctx, profileID)
	if err != nil {
		return nil, nil, err
	}
	browser, err := pw.Chromium.ConnectOverCDP(wsURL)
	if err != nil {
		_ = gpmClient.StopProfile(ctx, profileID)
		return nil, nil, fmt.Errorf("connectGPM: CDP connect failed: %w", err)
	}
	contexts := browser.Contexts()
	if len(contexts) == 0 {
		_ = browser.Close()
		_ = gpmClient.StopProfile(ctx, profileID)
		return nil, nil, fmt.Errorf("connectGPM: no browser context from GPM")
	}
	return browser, contexts[0], nil
}

// ConnectWithRetry attempts to connect to GPM with exponential backoff retries.
func (g *GPMService) ConnectWithRetry(ctx context.Context, pw *playwright.Playwright, gpmClient gpm.GPMClient, profileID string, log *zap.Logger, maxRetries int) (playwright.Browser, playwright.BrowserContext, error) {
	var browser playwright.Browser
	var context playwright.BrowserContext
	var err error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		browser, context, err = g.Connect(ctx, pw, gpmClient, profileID, log)
		if err == nil {
			return browser, context, nil
		}
		if browser != nil {
			_ = browser.Close()
		}

		// Stop the profile on failure — if context is cancelled, return immediately.
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		default:
		}
		_ = gpmClient.StopProfile(ctx, profileID)

		if attempt < maxRetries {
			// Exponential backoff: 1s → 2s → 4s
			backoff := backoffDuration(attempt)
			log.Sugar().Warnf("GPM reconnect attempt %d/%d, retrying in %v: %v", attempt, maxRetries, backoff, err)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			}
		}
	}
	return nil, nil, fmt.Errorf("connectGPMWithRetry: failed after %d attempts: %w", maxRetries, err)
}
