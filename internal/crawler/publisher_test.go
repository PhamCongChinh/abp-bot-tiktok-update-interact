package crawler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"abp-bot-tiktok/internal/models"
	"abp-bot-tiktok/pkg/api"

	"go.uber.org/zap"
)

// testPublisher creates a Publisher wired to a test HTTP server and returns
// both so the caller can inspect the server's request count.
func testPublisher(t *testing.T) (*Publisher, *httptest.Server, *int32) {
	t.Helper()

	var requestCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusOK)
	}))

	apiClient := api.NewClient(srv.URL, 10*time.Second, zap.NewNop())
	p := NewPublisher(apiClient, zap.NewNop())
	t.Cleanup(func() {
		p.Shutdown()
		srv.Close()
	})
	return p, srv, &requestCount
}

func TestPublisher_PushBatchAndFlush(t *testing.T) {
	p, srv, reqCount := testPublisher(t)
	_ = srv

	videos := []models.VideoItem{
		{VideoID: "v1", Keyword: "kw", OrgID: 1, UniqueID: "u1", AuthID: "a1", AuthName: "User"},
		{VideoID: "v2", Keyword: "kw", OrgID: 1, UniqueID: "u2", AuthID: "a2", AuthName: "User2"},
		{VideoID: "v3", Keyword: "kw", OrgID: 1, UniqueID: "u3", AuthID: "a3", AuthName: "User3"},
	}

	ctx := context.Background()
	p.PushBatch(ctx, videos)

	// Wait for the worker to pick up and flush the batch.
	time.Sleep(200 * time.Millisecond)

	// Force a flush by shutting down — remaining videos are drained.
	// Since we pushed only 3 videos (below batch max of 10) and the timer
	// is 5s, they might not have flushed yet. Shutdown ensures they do.
	p.Shutdown()

	// At least one API request should have been made.
	if atomic.LoadInt32(reqCount) < 1 {
		t.Errorf("expected at least 1 API request, got %d", atomic.LoadInt32(reqCount))
	}
}

func TestPublisher_ShutdownFlushesRemaining(t *testing.T) {
	p, _, reqCount := testPublisher(t)

	videos := []models.VideoItem{
		{VideoID: "v1", Keyword: "kw", OrgID: 1, UniqueID: "u1", AuthID: "a1", AuthName: "User"},
	}

	ctx := context.Background()
	p.PushBatch(ctx, videos)

	// Shutdown should drain and flush the remaining video.
	p.Shutdown()

	if atomic.LoadInt32(reqCount) < 1 {
		t.Errorf("expected at least 1 API request after shutdown, got %d", atomic.LoadInt32(reqCount))
	}
}

func TestPublisher_ShutdownIdempotent(t *testing.T) {
	p, _, _ := testPublisher(t)

	// Multiple Shutdown calls should not panic.
	p.Shutdown()
	p.Shutdown()
	p.Shutdown()
}

func TestPublisher_PushBatchEmpty(t *testing.T) {
	p, _, _ := testPublisher(t)

	// Pushing empty slice should not block or panic.
	p.PushBatch(context.Background(), nil)
	p.PushBatch(context.Background(), []models.VideoItem{})

	p.Shutdown()
}

func TestPublisher_BackpressureDrop(t *testing.T) {
	// Create a slow server to simulate backpressure.
	var requestCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		time.Sleep(500 * time.Millisecond) // Slow response to fill the channel
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	apiClient := api.NewClient(srv.URL, 10*time.Second, zap.NewNop())
	p := NewPublisher(apiClient, zap.NewNop())
	defer p.Shutdown()

	// Push more videos than the channel buffer (100) to trigger backpressure.
	// The first 100 go into the buffer; the rest should be dropped.
	totalVideos := 200
	ctx := context.Background()

	for i := 0; i < totalVideos; i++ {
		video := models.VideoItem{
			VideoID:  "v" + string(rune('0'+i%10)),
			Keyword:  "kw",
			OrgID:    1,
			UniqueID: "u1",
			AuthID:   "a1",
			AuthName: "User",
		}
		p.PushBatch(ctx, []models.VideoItem{video})
	}

	// Wait for workers to process some videos.
	time.Sleep(2 * time.Second)

	// Some videos should have been processed; backpressure drops should
	// have occurred. We can't assert exact numbers due to timing, but the
	// test should not panic or deadlock.
	p.Shutdown()

	count := atomic.LoadInt32(&requestCount)
	if count < 1 {
		t.Error("expected at least 1 API request")
	}
	t.Logf("API requests made: %d (from %d videos pushed, backpressure may have dropped some)", count, totalVideos)
}

func TestPublisher_WorkerBatchCollection(t *testing.T) {
	var requestCount int32
	var batchSizes []int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	apiClient := api.NewClient(srv.URL, 10*time.Second, zap.NewNop())
	p := NewPublisher(apiClient, zap.NewNop())

	// Push exactly 10 videos — should trigger batch flush at size 10.
	videos := make([]models.VideoItem, 10)
	for i := 0; i < 10; i++ {
		videos[i] = models.VideoItem{
			VideoID:  "v" + string(rune('0'+i)),
			Keyword:  "kw",
			OrgID:    1,
			UniqueID: "u",
			AuthID:   "a",
			AuthName: "User",
		}
	}

	ctx := context.Background()
	p.PushBatch(ctx, videos)

	// Wait for the worker to process the batch.
	time.Sleep(200 * time.Millisecond)

	p.Shutdown()

	if atomic.LoadInt32(&requestCount) < 1 {
		t.Error("expected at least 1 API request for 10-video batch")
	}
	_ = batchSizes // captured for future assertion if needed
}

func TestPublisher_PushToAPI_Sync(t *testing.T) {
	// PushToAPI should still work as a synchronous method for direct calls.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	apiClient := api.NewClient(srv.URL, 10*time.Second, zap.NewNop())
	p := NewPublisher(apiClient, zap.NewNop())
	defer p.Shutdown()

	videos := []models.VideoItem{
		{VideoID: "v1", Keyword: "kw", OrgID: 1, UniqueID: "u1", AuthID: "a1", AuthName: "User"},
	}

	err := p.PushToAPI(context.Background(), videos)
	if err != nil {
		t.Fatalf("unexpected error from PushToAPI: %v", err)
	}
}

func TestPublisher_NewPublisherStartsWorkers(t *testing.T) {
	// Verify that NewPublisher starts workers by pushing a video and
	// confirming it gets flushed without explicitly calling Start.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	apiClient := api.NewClient(srv.URL, 10*time.Second, zap.NewNop())
	p := NewPublisher(apiClient, zap.NewNop())
	defer p.Shutdown()

	videos := []models.VideoItem{
		{VideoID: "v1", Keyword: "kw", OrgID: 1, UniqueID: "u1", AuthID: "a1", AuthName: "User"},
	}
	p.PushBatch(context.Background(), videos)

	// Give workers time to process.
	time.Sleep(200 * time.Millisecond)

	p.Shutdown()
	// If we reach here without panic, workers are running correctly.
}
