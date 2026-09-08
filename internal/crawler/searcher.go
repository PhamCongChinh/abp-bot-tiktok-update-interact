package crawler

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"abp-bot-tiktok/internal/utils"
	"abp-bot-tiktok/pkg/config"
	"abp-bot-tiktok/pkg/gpm"

	"github.com/playwright-community/playwright-go"
	"go.uber.org/zap"
)

// Searcher orchestrates TikTok search crawling — navigating to search pages,
// scrolling, collecting API responses, parsing videos, and pushing to the
// backend API. This separates the search concern from the Crawler orchestrator.
type Searcher struct {
	cfg       *config.Config
	publisher *Publisher
	gpmSvc    *GPMService
	scraper   *Scraper
}

// NewSearcher creates a Searcher wired with all its dependencies.
func NewSearcher(cfg *config.Config, publisher *Publisher, gpmSvc *GPMService, scraper *Scraper) *Searcher {
	return &Searcher{
		cfg:       cfg,
		publisher: publisher,
		gpmSvc:    gpmSvc,
		scraper:   scraper,
	}
}

// CrawlSearch runs the full search crawl loop for a set of keywords within a
// single GPM profile session.
func (s *Searcher) CrawlSearch(ctx context.Context, pw *playwright.Playwright, gpmClient gpm.GPMClient, profileID string, keywords []string, sessionLog *zap.Logger, tag string) {
	sessionLog.Info("Crawl session started",
		zap.Int("total_keywords", len(keywords)),
		zap.String("profile_id", profileID[:8]),
	)

	total := len(keywords)
	i := 0
	pageCount := 0

	for i < total {
		select {
		case <-ctx.Done():
			sessionLog.Warn("crawlSearch: context cancelled, stopping crawl session")
			return
		default:
		}

		// Pagination guard: enforce per-session page limit.
		if s.cfg.MaxPagesPerSession > 0 && pageCount >= s.cfg.MaxPagesPerSession {
			sessionLog.Sugar().Warnf("reached maxPagesPerSession=%d", s.cfg.MaxPagesPerSession)
			return
		}
		pageCount++

		batchSize := utils.RandInt(s.cfg.BatchMin, s.cfg.BatchMax)
		batch := keywords[i:min(i+batchSize, total)]

		if !utils.WaitForResources(ctx, sessionLog, tag) {
			return
		}

		browser, bctx, err := s.gpmSvc.Connect(ctx, pw, gpmClient, profileID, sessionLog)
		if err != nil {
			sessionLog.Sugar().Warnf("%s GPM connect failed after retries: %v", tag, err)
			for _, keyword := range batch {
				sessionLog.Sugar().Infof("%s %q -> 0 videos pushed to API (GPM connect failed)", tag, keyword)
			}
			i += batchSize
			continue
		}

		// Dùng anonymous func + defer để đảm bảo browser luôn được đóng
		func() {
			defer func() { _ = browser.Close() }()
			defer func() { _ = gpmClient.StopProfile(ctx, profileID) }()

			for keywordIdx, keyword := range batch {
				select {
				case <-ctx.Done():
					sessionLog.Warn("crawlSearch: context cancelled during keyword batch")
					return
				default:
				}

				page, err := s.scraper.CreatePageWithRetry(bctx, 3, sessionLog)
				if err != nil {
					sessionLog.Sugar().Infof("%s %q -> 0 videos pushed to API", tag, keyword)
				} else {
					s.CrawlKeyword(ctx, page, keyword, sessionLog, tag)
					_ = page.Close()
				}

				if keywordIdx < len(batch)-1 {
					sleepSec := utils.RandInt(s.cfg.SleepMinKeyword, s.cfg.SleepMaxKeyword)
					select {
					case <-time.After(time.Duration(sleepSec) * time.Second):
					case <-ctx.Done():
						return
					}
				}
			}
		}()

		i += batchSize
		if i < total {
			restSec := utils.RandInt(s.cfg.RestMinSession, s.cfg.RestMaxSession)
			select {
			case <-time.After(time.Duration(restSec) * time.Second):
			case <-ctx.Done():
				return
			}
		}
	}
}

// CrawlKeyword crawls TikTok search results for a single keyword: navigates to
// the search page, scrolls to load videos, collects API response items, parses
// them into VideoItem models, and pushes to the backend API.
func (s *Searcher) CrawlKeyword(ctx context.Context, page playwright.Page, keyword string, log *zap.Logger, tag string) {
	// Images and media are left un-blocked: TikTok is thumbnail-heavy and
	// RandomViewVideo simulates actually watching a video, so a session that
	// never loads any image/video bytes is an obvious bot signal to
	// server-side traffic analysis. Only non-visual resource types are
	// skipped for bandwidth.
	_ = page.Route("**/*", func(route playwright.Route) {
		switch route.Request().ResourceType() {
		case "stylesheet", "font", "other":
			_ = route.Abort()
		default:
			_ = route.Continue()
		}
	})

	var mu sync.Mutex
	var collectedItems []map[string]any

	page.On("response", func(res playwright.Response) {
		if !containsAny(res.URL(), []string{searchAPI}) {
			return
		}
		go func(res playwright.Response) {
			var body map[string]any
			if err := res.JSON(&body); err != nil || body == nil {
				log.Sugar().Warnf("%s %q -> failed to parse search API response: %v", tag, keyword, err)
				return
			}
			statusCode, _ := body["status_code"].(float64)
			items, _ := body["item_list"].([]any)

			// Detect rate limit / captcha
			if statusCode == 2061 || statusCode == 10000 || statusCode == -1 {
				log.Sugar().Warnf("%s %q -> rate limit detected (status=%v), pausing 5 minutes", tag, keyword, statusCode)
				select {
				case <-time.After(5 * time.Minute):
				case <-ctx.Done():
				}
				return
			}

			if statusCode != 0 {
				return
			}

			mu.Lock()
			defer mu.Unlock()
			seen := make(map[string]bool)
			for _, existing := range collectedItems {
				if id, ok := existing["id"].(string); ok {
					seen[id] = true
				}
			}
			for _, raw := range items {
				item, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				id, _ := item["id"].(string)
				if id == "" || seen[id] {
					continue
				}
				seen[id] = true
				collectedItems = append(collectedItems, item)
			}
		}(res)
	})

	if _, err := page.Goto(tiktokURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   playwright.Float(30000),
	}); err != nil {
		log.Sugar().Infof("%s %q -> 0 videos pushed to API", tag, keyword)
		return
	}
	_, _ = page.Evaluate("window.moveTo(0, 0); window.resizeTo(screen.availWidth, screen.availHeight);")
	utils.Sleep(4000, 7000)
	if err := utils.RandomMouseMove(page); err != nil {
		log.Sugar().Warnf("%s %q -> RandomMouseMove error (non-critical): %v", tag, keyword, err)
	}
	utils.Sleep(500, 1500)

	// Browse the For You feed briefly before searching — landing on the
	// homepage and jumping straight to a search URL with zero feed
	// interaction is an easy signal to spot as automated.
	if err := utils.HumanScroll(page, utils.RandInt(2, 4)); err != nil {
		log.Sugar().Warnf("%s %q -> home feed scroll error (non-critical): %v", tag, keyword, err)
	}
	utils.Sleep(800, 2000)

	encoded := url.QueryEscape(keyword)
	ts := time.Now().UnixMilli()

	// Navigate to Top tab first
	topURL := fmt.Sprintf("%s/search?q=%s&t=%d", tiktokURL, encoded, ts)
	if _, err := page.Goto(topURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   playwright.Float(30000),
	}); err != nil {
		log.Sugar().Infof("%s %q -> 0 videos pushed to API", tag, keyword)
		return
	}
	utils.Sleep(3000, 5000)

	// Check if Top tab has videos, if not switch to Video tab
	hasVideos, _ := page.Evaluate(`() => {
		const items = document.querySelectorAll('[data-e2e="search-common-video"], [class*="DivItemContainer"]');
		return items.length > 0;
	}`)
	if hasVideos == nil || hasVideos == false {
		videoURL := fmt.Sprintf("%s/search/video?q=%s&t=%d", tiktokURL, encoded, ts)
		if _, err := page.Goto(videoURL, playwright.PageGotoOptions{
			WaitUntil: playwright.WaitUntilStateDomcontentloaded,
			Timeout:   playwright.Float(30000),
		}); err != nil {
			log.Sugar().Infof("%s %q -> 0 videos pushed to API", tag, keyword)
			return
		}
		utils.Sleep(4000, 6000)
	}

	// Scroll to load more videos
	scrollTimes := utils.RandInt(10, 15)
	if err := utils.HumanScroll(page, scrollTimes); err != nil {
		log.Sugar().Warnf("%s %q -> HumanScroll error (non-critical): %v", tag, keyword, err)
	}

	// Random view video
	if err := utils.RandomViewVideo(page); err != nil {
		log.Sugar().Warnf("%s %q -> RandomViewVideo error (non-critical): %v", tag, keyword, err)
	}
	utils.Sleep(1500, 2500)

	mu.Lock()
	items := collectedItems
	mu.Unlock()

	orgID := s.cfg.KeywordOrgMap[keyword]
	results := s.publisher.ParseVideos(keyword, orgID, items)

	// Pagination guard: enforce per-keyword video limit.
	if s.cfg.MaxVideosPerKeyword > 0 && len(results) > s.cfg.MaxVideosPerKeyword {
		log.Sugar().Warnf("reached maxVideosPerKeyword=%d for keyword=%s", s.cfg.MaxVideosPerKeyword, keyword)
		results = results[:s.cfg.MaxVideosPerKeyword]
	}

	if len(results) > 0 {
		s.publisher.PushBatch(ctx, results)
	}
	log.Sugar().Infof("%s %q -> %d videos queued for API push", tag, keyword, len(results))
}
