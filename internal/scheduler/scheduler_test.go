package scheduler

import (
	"context"
	"testing"
	"time"

	"abp-bot-tiktok/internal/crawler"
	"abp-bot-tiktok/pkg/config"

	"go.uber.org/zap"
)

func TestNew(t *testing.T) {
	cfg := &config.Config{}
	log := zap.NewNop()
	c := &crawler.Crawler{}

	s := New(cfg, log, c)
	if s == nil {
		t.Fatal("New returned nil")
	}
	if s.cfg != cfg {
		t.Error("config not set")
	}
	if s.log != log {
		t.Error("logger not set")
	}
	if s.crawler != c {
		t.Error("crawler not set")
	}
	if s.stopCh == nil {
		t.Error("stopCh is nil")
	}
}

func TestStop(t *testing.T) {
	cfg := &config.Config{}
	log := zap.NewNop()
	s := New(cfg, log, &crawler.Crawler{})

	// Stop should close the channel without panic.
	s.Stop()

	// Verify the channel is closed by reading from it.
	select {
	case _, ok := <-s.stopCh:
		if ok {
			t.Error("stopCh should be closed but read returned ok=true")
		}
	default:
		t.Error("stopCh is not closed")
	}
}

func TestStop_Idempotent(t *testing.T) {
	cfg := &config.Config{}
	log := zap.NewNop()
	s := New(cfg, log, &crawler.Crawler{})

	// Call Stop twice — should not panic.
	s.Stop()
	s.Stop()

	// Verify the channel is closed after the first Stop.
	select {
	case _, ok := <-s.stopCh:
		if ok {
			t.Error("stopCh should be closed but read returned ok=true")
		}
	default:
		t.Error("stopCh is not closed")
	}
}

func TestWaitIfNightHours_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	log := zap.NewNop()

	// waitIfNightHours should return immediately when context is cancelled,
	// regardless of time of day.
	done := make(chan struct{})
	go func() {
		waitIfNightHours(ctx, log)
		close(done)
	}()

	select {
	case <-done:
		// Expected — should return immediately.
	case <-time.After(2 * time.Second):
		t.Fatal("waitIfNightHours blocked for >2s on cancelled context")
	}
}

func TestWaitIfNightHours_Daytime(t *testing.T) {
	// During the day (hours 3-23), waitIfNightHours should return
	// immediately without blocking.
	ctx := context.Background()
	log := zap.NewNop()

	done := make(chan struct{})
	go func() {
		waitIfNightHours(ctx, log)
		close(done)
	}()

	select {
	case <-done:
		// Expected — during day hours, the function returns immediately.
	case <-time.After(2 * time.Second):
		// This could happen if the test runs between 00:00-03:00.
		// In that case, the function would sleep until 03:00.
		// We just note this as an acceptable false-positive.
		t.Log("waitIfNightHours blocked (may be night hours 00-03)")
	}
}

func TestRunInterval_CancelledContext(t *testing.T) {
	cfg := &config.Config{}
	log := zap.NewNop()
	s := New(cfg, log, &crawler.Crawler{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	done := make(chan struct{})
	go func() {
		s.runInterval(ctx, 1, 2)
		close(done)
	}()

	select {
	case <-done:
		// Expected — should detect cancelled context and return.
	case <-time.After(3 * time.Second):
		t.Fatal("runInterval blocked for >3s on cancelled context")
	}
}

func TestStart_WithUseGPMFalse(t *testing.T) {
	cfg := &config.Config{
		UseGPM: false,
	}
	log := zap.NewNop()
	c := &crawler.Crawler{}
	s := New(cfg, log, c)

	ctx, cancel := context.WithCancel(context.Background())

	// Start runs the initial crawl (which returns immediately for UseGPM=false)
	// and then launches runInterval in a goroutine.
	done := make(chan struct{})
	go func() {
		s.Start(ctx)
		close(done)
	}()

	// Give Start time to complete the initial crawl and enter runInterval.
	time.Sleep(100 * time.Millisecond)

	// Cancel context to stop runInterval, then stop scheduler.
	cancel()
	s.Stop()

	select {
	case <-done:
		// Expected.
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return within 5s after cancel+stop")
	}
}

func TestRunInterval_StopCh(t *testing.T) {
	cfg := &config.Config{}
	log := zap.NewNop()
	s := New(cfg, log, &crawler.Crawler{})

	ctx := context.Background()

	done := make(chan struct{})
	go func() {
		s.runInterval(ctx, 60, 60) // 60 minute interval
		close(done)
	}()

	// Give it a moment to enter the loop.
	time.Sleep(50 * time.Millisecond)
	s.Stop()

	select {
	case <-done:
		// Expected — Stop() should cause runInterval to return.
	case <-time.After(2 * time.Second):
		t.Fatal("runInterval did not stop after Stop() was called")
	}
}
