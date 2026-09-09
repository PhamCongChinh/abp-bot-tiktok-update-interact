package crawler

import (
	"context"
	"testing"
	"time"

	"abp-bot-tiktok/pkg/config"

	"go.uber.org/zap"
)

func TestNewURLVisitor(t *testing.T) {
	cfg := &config.Config{}
	gpmSvc := NewDefaultGPMService()
	scrpr := NewScraper()

	v := NewURLVisitor(cfg, gpmSvc, scrpr, nil)

	if v == nil {
		t.Fatal("NewURLVisitor returned nil")
	}
	if v.cfg != cfg {
		t.Error("config not set correctly")
	}
	if v.gpmSvc != gpmSvc {
		t.Error("gpmSvc not set correctly")
	}
	if v.scraper != scrpr {
		t.Error("scraper not set correctly")
	}
}

func TestCrawlURLs_ContextCancelled(t *testing.T) {
	cfg := &config.Config{
		MaxPagesPerSession: 10,
	}
	log := zap.NewNop()
	gpmSvc := NewDefaultGPMService()
	scrpr := NewScraper()

	v := NewURLVisitor(cfg, gpmSvc, scrpr, nil)

	// When context is already cancelled, CrawlURLs should return immediately
	// without making any GPM connections or page interactions.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	// nil playwright and gpmClient — these should not be accessed since
	// context is already done.
	v.CrawlURLs(ctx, nil, nil, "profile-12345", []string{"url1", "url2"}, log, "test-tag")
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Errorf("CrawlURLs with cancelled context took %v, expected < 500ms", elapsed)
	}
}

func TestCrawlURLs_EmptyURLs(t *testing.T) {
	cfg := &config.Config{}
	log := zap.NewNop()
	gpmSvc := NewDefaultGPMService()
	scrpr := NewScraper()

	v := NewURLVisitor(cfg, gpmSvc, scrpr, nil)

	ctx := context.Background()
	start := time.Now()

	// Empty URL list — loop should never enter.
	v.CrawlURLs(ctx, nil, nil, "profile-12345", nil, log, "test-tag")
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Errorf("CrawlURLs with nil urls took %v, expected < 500ms", elapsed)
	}

	// Also test with explicit empty slice.
	v.CrawlURLs(ctx, nil, nil, "profile-12345", []string{}, log, "test-tag")
}

func TestCrawlURLs_MaxPagesGuard(t *testing.T) {
	// When MaxPagesPerSession is 1, CrawlURLs should stop after 1 page load
	// attempt, before actually loading a page (since GPM won't connect
	// without a real server, it will fail the connect and move to the next
	// batch).
	cfg := &config.Config{
		MaxPagesPerSession: 1,
		BatchMin:           2,
		BatchMax:           2,
	}
	log := zap.NewNop()
	gpmSvc := NewDefaultGPMService()
	scrpr := NewScraper()

	v := NewURLVisitor(cfg, gpmSvc, scrpr, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	start := time.Now()
	// mockGPMClient (from gpm_test.go) is unconfigured — StartProfile errors
	// immediately, so GPM connect fails without ever touching a nil pw.
	v.CrawlURLs(ctx, nil, &mockGPMClient{}, "profile-12345", []string{"u1", "u2", "u3", "u4"}, log, "test-tag")
	elapsed := time.Since(start)

	t.Logf("CrawlURLs with MaxPagesPerSession=1 completed in %v", elapsed)
}

func TestCrawlURLs_Constants(t *testing.T) {
	if tiktokURL != "https://www.tiktok.com" {
		t.Errorf("tiktokURL = %q, want %q", tiktokURL, "https://www.tiktok.com")
	}
	if itemDetailAPI != "/api/item_detail/" {
		t.Errorf("itemDetailAPI = %q, want %q", itemDetailAPI, "/api/item_detail/")
	}
	if rehydrationScriptID != "__UNIVERSAL_DATA_FOR_REHYDRATION__" {
		t.Errorf("rehydrationScriptID = %q, want %q", rehydrationScriptID, "__UNIVERSAL_DATA_FOR_REHYDRATION__")
	}
}

func TestParseItemStruct(t *testing.T) {
	item := map[string]any{
		"id":         "full-video-1",
		"desc":       "Full test description",
		"createTime": float64(1700000000),
		"author": map[string]any{
			"uniqueId": "creator1",
			"id":       "auth-001",
			"nickname": "Creator One",
		},
		"stats": map[string]any{
			"commentCount": float64(100),
			"shareCount":   float64(200),
			"diggCount":    float64(300),
			"collectCount": float64(50),
			"playCount":    float64(10000),
		},
	}

	v := parseItemStruct(42, item)

	if v.OrgID != 42 {
		t.Errorf("OrgID = %d, want 42", v.OrgID)
	}
	if v.VideoID != "full-video-1" {
		t.Errorf("VideoID = %q, want %q", v.VideoID, "full-video-1")
	}
	if v.Description != "Full test description" {
		t.Errorf("Description = %q, want %q", v.Description, "Full test description")
	}
	if v.PubTime != 1700000000 {
		t.Errorf("PubTime = %d, want 1700000000", v.PubTime)
	}
	if v.UniqueID != "creator1" {
		t.Errorf("UniqueID = %q, want %q", v.UniqueID, "creator1")
	}
	if v.AuthID != "auth-001" {
		t.Errorf("AuthID = %q, want %q", v.AuthID, "auth-001")
	}
	if v.AuthName != "Creator One" {
		t.Errorf("AuthName = %q, want %q", v.AuthName, "Creator One")
	}
	if v.Comments != 100 || v.Shares != 200 || v.Reactions != 300 || v.Favors != 50 || v.Views != 10000 {
		t.Errorf("stats mismatch: %+v", v)
	}
}

func TestParseItemStruct_StringNumbers(t *testing.T) {
	// TikTok's embedded rehydration JSON serializes large counters as
	// strings to avoid JS float precision loss.
	item := map[string]any{
		"id":         "video-2",
		"createTime": "1700000000",
		"stats": map[string]any{
			"commentCount": "10",
			"shareCount":   "20",
			"diggCount":    "30",
			"collectCount": "5",
			"playCount":    "1000",
		},
	}

	v := parseItemStruct(1, item)

	if v.PubTime != 1700000000 {
		t.Errorf("PubTime = %d, want 1700000000", v.PubTime)
	}
	if v.Comments != 10 || v.Shares != 20 || v.Reactions != 30 || v.Favors != 5 || v.Views != 1000 {
		t.Errorf("stats mismatch: %+v", v)
	}
}

func TestParseItemStruct_MissingAuthorStats(t *testing.T) {
	item := map[string]any{
		"id":   "no-nested-maps",
		"desc": "minimal item",
	}

	v := parseItemStruct(1, item)

	if v.VideoID != "no-nested-maps" {
		t.Errorf("VideoID = %q, want %q", v.VideoID, "no-nested-maps")
	}
	if v.AuthName != "" {
		t.Errorf("expected empty AuthName, got %q", v.AuthName)
	}
	if v.Views != 0 {
		t.Errorf("expected 0 Views, got %d", v.Views)
	}
}

func TestFlexInt64(t *testing.T) {
	tests := []struct {
		name string
		v    any
		want int64
	}{
		{"float64", float64(42), 42},
		{"numeric string", "42", 42},
		{"float string", "42.9", 42},
		{"invalid string", "not-a-number", 0},
		{"nil", nil, 0},
		{"bool", true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := flexInt64(tt.v)
			if got != tt.want {
				t.Errorf("flexInt64(%v) = %d, want %d", tt.v, got, tt.want)
			}
		})
	}
}
