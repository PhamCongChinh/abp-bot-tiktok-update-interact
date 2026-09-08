package crawler

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"abp-bot-tiktok/internal/repository"
	"abp-bot-tiktok/internal/utils"
	"abp-bot-tiktok/pkg/api"
	"abp-bot-tiktok/pkg/config"
	"abp-bot-tiktok/pkg/gpm"
	"abp-bot-tiktok/pkg/logger"

	"github.com/playwright-community/playwright-go"
	"go.uber.org/zap"
)

const (
	tiktokURL = "https://www.tiktok.com"
)

// Crawler is the top-level orchestrator that coordinates direct video-URL
// visits across multiple GPM profiles. Each sub-concern (GPM connections,
// page scraping, video parsing/publishing, visit loops) is delegated to a
// dedicated service.
type Crawler struct {
	cfg       *config.Config
	log       *zap.Logger
	videoRepo repository.VideoStore
	apiClient api.APIClient
	gpmSvc    *GPMService
	scraper   *Scraper
	publisher *Publisher
	visitor   *URLVisitor
}

// New creates a fully wired Crawler with all sub-services.
func New(cfg *config.Config, log *zap.Logger, videoRepo repository.VideoStore) *Crawler {
	var apiClient api.APIClient
	if cfg.APIURL != "" {
		apiClient = api.NewClient(cfg.APIURL, time.Duration(cfg.HTTPTimeoutSeconds)*time.Second, log)
	}
	gpmSvc := NewGPMService()
	scraper := NewScraper()
	publisher := NewPublisher(apiClient, log)
	visitor := NewURLVisitor(cfg, gpmSvc, scraper)

	return &Crawler{
		cfg:       cfg,
		log:       log,
		videoRepo: videoRepo,
		apiClient: apiClient,
		gpmSvc:    gpmSvc,
		scraper:   scraper,
		publisher: publisher,
		visitor:   visitor,
	}
}

// Run starts URL-visit crawling across all configured GPM profiles.
func (c *Crawler) Run(ctx context.Context) {
	if c.cfg == nil || c.log == nil {
		if c.log != nil {
			c.log.Error("Crawler.Run: nil config")
		}
		return
	}
	if !c.cfg.UseGPM {
		c.log.Error("GPM config required. Set GPM_API and PROFILE_IDS in .env")
		return
	}

	urls := make([]string, len(c.cfg.PostURLs))
	copy(urls, c.cfg.PostURLs)
	rand.Shuffle(len(urls), func(i, j int) {
		urls[i], urls[j] = urls[j], urls[i]
	})

	c.log.Info("Crawl cycle: URLs to visit",
		zap.Int("total_urls", len(urls)),
	)

	numProfiles := len(c.cfg.ProfileIDs)
	chunks := splitURLs(urls, numProfiles)

	for i, profileID := range c.cfg.ProfileIDs {
		c.log.Info("URL assignment",
			zap.String("profile_id", profileID),
			zap.Int("url_count", len(chunks[i])),
		)
	}

	var wg sync.WaitGroup
launch:
	for i, profileID := range c.cfg.ProfileIDs {
		wg.Add(1)
		go func(profileID string, urls []string, idx int) {
			defer wg.Done()
			c.runProfile(ctx, profileID, urls, idx)
		}(profileID, chunks[i], i)

		if i < numProfiles-1 {
			staggerSec := utils.RandInt(15, 45)
			select {
			case <-time.After(time.Duration(staggerSec) * time.Second):
			case <-ctx.Done():
				break launch
			}
		}
	}
	wg.Wait()
}

// splitURLs distributes URLs across n profiles in a round-robin fashion.
func splitURLs(urls []string, n int) [][]string {
	chunks := make([][]string, n)
	for i, u := range urls {
		chunks[i%n] = append(chunks[i%n], u)
	}
	return chunks
}

// runProfile runs the crawl for a single GPM profile.
func (c *Crawler) runProfile(ctx context.Context, profileID string, urls []string, idx int) {
	tag := fmt.Sprintf("[P%d|%s...]", idx+1, profileID[:8])

	// Generate session ID for this profile run and use session-aware logger.
	sessionID := logger.NewSessionID()
	sessionLog := logger.WithSession(c.log, sessionID)
	sessionLog.Info("Profile run started",
		zap.String("tag", tag),
		zap.String("profile_id", profileID),
		zap.Int("total_urls", len(urls)),
	)

	pw, err := playwright.Run()
	if err != nil {
		sessionLog.Sugar().Errorf("%s playwright error: %v", tag, err)
		return
	}
	defer func() { _ = pw.Stop() }()

	gpmClient := gpm.NewClient(c.cfg.GPMAPI, sessionLog)
	c.visitor.CrawlURLs(ctx, pw, gpmClient, profileID, urls, sessionLog, tag)
}

// Utility functions shared across the crawler package.

// containsAny returns true if s contains any of the substrings in subs.
func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
	}
	return false
}

// toString converts an arbitrary value to its string representation.
func toString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// toFloat converts an arbitrary value to float64, returning 0 for non-float values.
func toFloat(v any) float64 {
	if v == nil {
		return 0
	}
	if f, ok := v.(float64); ok {
		return f
	}
	return 0
}

// mapGet safely retrieves a value from a map, returning nil if the map is nil.
func mapGet(m map[string]any, key string) any {
	if m == nil {
		return nil
	}
	return m[key]
}

// min returns the smaller of a and b.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// backoffDuration returns the exponential backoff duration for the given
// retry attempt: 1s → 2s → 4s → 8s → ...
func backoffDuration(attempt int) time.Duration {
	return time.Duration(1<<(attempt-1)) * time.Second
}

// Shutdown gracefully shuts down the Crawler's publisher, draining any
// buffered videos and flushing the final batch to the API.
func (c *Crawler) Shutdown() {
	if c.publisher != nil {
		c.log.Info("Crawler: shutting down publisher...")
		c.publisher.Shutdown()
		c.log.Info("Crawler: publisher shutdown complete")
	}
}
