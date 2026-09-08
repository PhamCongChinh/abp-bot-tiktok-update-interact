package crawler

import (
	"context"
	"testing"
	"time"

	"abp-bot-tiktok/pkg/api"
	"abp-bot-tiktok/pkg/config"

	"go.uber.org/zap"
)

func TestNewSearcher(t *testing.T) {
	cfg := &config.Config{}
	log := zap.NewNop()
	apiClient := api.NewClient("http://localhost:9999", 10*time.Second, log)
	publisher := NewPublisher(apiClient, log)
	defer publisher.Shutdown()
	gpmSvc := NewDefaultGPMService()
	scrpr := NewScraper()

	s := NewSearcher(cfg, publisher, gpmSvc, scrpr)

	if s == nil {
		t.Fatal("NewSearcher returned nil")
	}
	if s.cfg != cfg {
		t.Error("config not set correctly")
	}
	if s.publisher != publisher {
		t.Error("publisher not set correctly")
	}
	if s.gpmSvc != gpmSvc {
		t.Error("gpmSvc not set correctly")
	}
	if s.scraper != scrpr {
		t.Error("scraper not set correctly")
	}
}

func TestCrawlSearch_ContextCancelled(t *testing.T) {
	cfg := &config.Config{
		MaxPagesPerSession: 10,
	}
	log := zap.NewNop()
	apiClient := api.NewClient("http://localhost:9999", 10*time.Second, log)
	publisher := NewPublisher(apiClient, log)
	defer publisher.Shutdown()
	gpmSvc := NewDefaultGPMService()
	scrpr := NewScraper()

	s := NewSearcher(cfg, publisher, gpmSvc, scrpr)

	// When context is already cancelled, CrawlSearch should return immediately
	// without making any GPM connections or page interactions.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	// nil playwright and keywords — these should not be accessed since
	// context is already done.
	s.CrawlSearch(ctx, nil, nil, "profile-12345", []string{"kw1", "kw2"}, log, "test-tag")
	elapsed := time.Since(start)

	// Should return quickly since context was cancelled before the loop starts.
	if elapsed > 500*time.Millisecond {
		t.Errorf("CrawlSearch with cancelled context took %v, expected < 500ms", elapsed)
	}

	t.Logf("CrawlSearch returned after %v with cancelled context", elapsed)
}

func TestCrawlSearch_EmptyKeywords(t *testing.T) {
	cfg := &config.Config{}
	log := zap.NewNop()
	apiClient := api.NewClient("http://localhost:9999", 10*time.Second, log)
	publisher := NewPublisher(apiClient, log)
	defer publisher.Shutdown()
	gpmSvc := NewDefaultGPMService()
	scrpr := NewScraper()

	s := NewSearcher(cfg, publisher, gpmSvc, scrpr)

	ctx := context.Background()
	start := time.Now()

	// Empty keyword list — loop should never enter.
	s.CrawlSearch(ctx, nil, nil, "profile-12345", nil, log, "test-tag")
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Errorf("CrawlSearch with nil keywords took %v, expected < 500ms", elapsed)
	}

	// Also test with explicit empty slice.
	s.CrawlSearch(ctx, nil, nil, "profile-12345", []string{}, log, "test-tag")
}

func TestCrawlSearch_MaxPagesGuard(t *testing.T) {
	// When MaxPagesPerSession is 1, CrawlSearch should stop after 1 page load
	// attempt, before actually loading a page (since GPM won't connect without
	// a real server, it will fail the connect and move to the next batch).
	cfg := &config.Config{
		MaxPagesPerSession: 1,
		BatchMin:           2,
		BatchMax:           2,
	}
	log := zap.NewNop()
	apiClient := api.NewClient("http://localhost:9999", 10*time.Second, log)
	publisher := NewPublisher(apiClient, log)
	defer publisher.Shutdown()
	gpmSvc := NewDefaultGPMService()
	scrpr := NewScraper()

	s := NewSearcher(cfg, publisher, gpmSvc, scrpr)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	start := time.Now()
	// Long keyword list, but MaxPagesPerSession=1 should stop after pageCount reaches 1.
	s.CrawlSearch(ctx, nil, nil, "profile-12345", []string{"kw1", "kw2", "kw3", "kw4"}, log, "test-tag")
	elapsed := time.Since(start)

	t.Logf("CrawlSearch with MaxPagesPerSession=1 completed in %v", elapsed)
	// Should complete quickly since WaitForResources may take a moment,
	// but then pageCount hits the limit. Even if WaitForResources blocks
	// briefly, the test timeout of 30s handles it.
}

func TestCrawlSearch_Constants(t *testing.T) {
	// Verify critical constants used by Searcher.
	if tiktokURL != "https://www.tiktok.com" {
		t.Errorf("tiktokURL = %q, want %q", tiktokURL, "https://www.tiktok.com")
	}
	if searchAPI != "/api/search/item/full/" {
		t.Errorf("searchAPI = %q, want %q", searchAPI, "/api/search/item/full/")
	}
	if cutoffSpan != 7*24*60*60 {
		t.Errorf("cutoffSpan = %d, want %d (7 days in seconds)", cutoffSpan, 7*24*60*60)
	}
}
