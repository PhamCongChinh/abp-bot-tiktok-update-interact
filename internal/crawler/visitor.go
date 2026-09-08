package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"abp-bot-tiktok/internal/models"
	"abp-bot-tiktok/internal/utils"
	"abp-bot-tiktok/pkg/config"
	"abp-bot-tiktok/pkg/gpm"

	"github.com/playwright-community/playwright-go"
	"go.uber.org/zap"
)

// URLVisitor orchestrates direct TikTok video-page visits — navigating to a
// known video URL, scraping its current stats, and logging the result. This
// replaces the old keyword-search crawl (see git history for Searcher) with a
// re-visit flow driven by URLs already known to the backend (tbl_posts).
type URLVisitor struct {
	cfg     *config.Config
	gpmSvc  *GPMService
	scraper *Scraper
}

// NewURLVisitor creates a URLVisitor wired with its dependencies.
func NewURLVisitor(cfg *config.Config, gpmSvc *GPMService, scraper *Scraper) *URLVisitor {
	return &URLVisitor{
		cfg:     cfg,
		gpmSvc:  gpmSvc,
		scraper: scraper,
	}
}

// CrawlURLs runs the full visit loop for a set of video URLs within a single
// GPM profile session.
func (v *URLVisitor) CrawlURLs(ctx context.Context, pw *playwright.Playwright, gpmClient gpm.GPMClient, profileID string, urls []string, sessionLog *zap.Logger, tag string) {
	sessionLog.Info("Crawl session started",
		zap.Int("total_urls", len(urls)),
		zap.String("profile_id", profileID[:8]),
	)

	total := len(urls)
	i := 0
	pageCount := 0

	for i < total {
		select {
		case <-ctx.Done():
			sessionLog.Warn("CrawlURLs: context cancelled, stopping crawl session")
			return
		default:
		}

		// Pagination guard: enforce per-session page limit.
		if v.cfg.MaxPagesPerSession > 0 && pageCount >= v.cfg.MaxPagesPerSession {
			sessionLog.Sugar().Warnf("reached maxPagesPerSession=%d", v.cfg.MaxPagesPerSession)
			return
		}
		pageCount++

		batchSize := utils.RandInt(v.cfg.BatchMin, v.cfg.BatchMax)
		batch := urls[i:min(i+batchSize, total)]

		if !utils.WaitForResources(ctx, sessionLog, tag) {
			return
		}

		browser, bctx, err := v.gpmSvc.Connect(ctx, pw, gpmClient, profileID, sessionLog)
		if err != nil {
			sessionLog.Sugar().Warnf("%s GPM connect failed after retries: %v", tag, err)
			for _, videoURL := range batch {
				sessionLog.Sugar().Infof("%s %s -> skipped (GPM connect failed)", tag, videoURL)
			}
			i += batchSize
			continue
		}

		// Dùng anonymous func + defer để đảm bảo browser luôn được đóng
		func() {
			defer func() { _ = browser.Close() }()
			defer func() { _ = gpmClient.StopProfile(ctx, profileID) }()

			for urlIdx, videoURL := range batch {
				select {
				case <-ctx.Done():
					sessionLog.Warn("CrawlURLs: context cancelled during url batch")
					return
				default:
				}

				page, err := v.scraper.CreatePageWithRetry(bctx, 3, sessionLog)
				if err != nil {
					sessionLog.Sugar().Infof("%s %s -> skipped (page create failed)", tag, videoURL)
				} else {
					video, ok := v.VisitURL(ctx, page, videoURL, sessionLog, tag)
					_ = page.Close()
					if ok {
						sessionLog.Info("video data scraped",
							zap.String("tag", tag),
							zap.String("url", videoURL),
							zap.String("video_id", video.VideoID),
							zap.String("author", video.AuthName),
							zap.Int64("pub_time", video.PubTime),
							zap.Int64("views", video.Views),
							zap.Int64("comments", video.Comments),
							zap.Int64("shares", video.Shares),
							zap.Int64("reactions", video.Reactions),
							zap.Int64("favors", video.Favors),
						)
					}
				}

				if urlIdx < len(batch)-1 {
					sleepSec := utils.RandInt(v.cfg.SleepMinKeyword, v.cfg.SleepMaxKeyword)
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
			restSec := utils.RandInt(v.cfg.RestMinSession, v.cfg.RestMaxSession)
			select {
			case <-time.After(time.Duration(restSec) * time.Second):
			case <-ctx.Done():
				return
			}
		}
	}
}

// itemDetailAPI is the XHR TikTok fires when a video-detail page loads.
// rehydrationScriptID is the fallback: TikTok also embeds the same item data
// in a page-hydration JSON blob, used when no matching XHR is captured (e.g.
// a cached navigation).
const (
	itemDetailAPI       = "/api/item_detail/"
	rehydrationScriptID = "__UNIVERSAL_DATA_FOR_REHYDRATION__"
)

// VisitURL navigates directly to a TikTok video URL, waits for its stats to
// load, and extracts them into a VideoItem. Returns (video, false) if no
// video data could be extracted (page failed to load, video removed, TikTok
// changed its response shape, etc.) — the caller only logs on true.
func (v *URLVisitor) VisitURL(ctx context.Context, page playwright.Page, videoURL string, log *zap.Logger, tag string) (models.VideoItem, bool) {
	// Images and media are left un-blocked — see CrawlKeyword's historical
	// comment: a session that never loads visual bytes is an obvious bot
	// signal. Only non-visual resource types are skipped for bandwidth.
	_ = page.Route("**/*", func(route playwright.Route) {
		switch route.Request().ResourceType() {
		case "stylesheet", "font", "other":
			_ = route.Abort()
		default:
			_ = route.Continue()
		}
	})

	var mu sync.Mutex
	var itemStruct map[string]any

	page.On("response", func(res playwright.Response) {
		if !containsAny(res.URL(), []string{itemDetailAPI}) {
			return
		}
		go func(res playwright.Response) {
			var body map[string]any
			if err := res.JSON(&body); err != nil || body == nil {
				log.Sugar().Warnf("%s %s -> failed to parse item_detail response: %v", tag, videoURL, err)
				return
			}
			statusCode, _ := body["statusCode"].(float64)
			if statusCode == 2061 || statusCode == 10000 || statusCode == -1 {
				log.Sugar().Warnf("%s %s -> rate limit detected (status=%v), pausing 5 minutes", tag, videoURL, statusCode)
				select {
				case <-time.After(5 * time.Minute):
				case <-ctx.Done():
				}
				return
			}
			if statusCode != 0 {
				return
			}
			info, _ := body["itemInfo"].(map[string]any)
			item, _ := info["itemStruct"].(map[string]any)
			if item == nil {
				return
			}

			mu.Lock()
			if itemStruct == nil {
				itemStruct = item
			}
			mu.Unlock()
		}(res)
	})

	// Warm up on the home feed before jumping to the video — landing cold on
	// a video URL with zero feed interaction is an easy automation signal.
	if _, err := page.Goto(tiktokURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   playwright.Float(30000),
	}); err != nil {
		log.Sugar().Infof("%s %s -> visit failed (home nav): %v", tag, videoURL, err)
		return models.VideoItem{}, false
	}
	_, _ = page.Evaluate("window.moveTo(0, 0); window.resizeTo(screen.availWidth, screen.availHeight);")
	utils.Sleep(4000, 7000)
	if err := utils.RandomMouseMove(page); err != nil {
		log.Sugar().Warnf("%s %s -> RandomMouseMove error (non-critical): %v", tag, videoURL, err)
	}
	utils.Sleep(500, 1500)
	if err := utils.HumanScroll(page, utils.RandInt(2, 4)); err != nil {
		log.Sugar().Warnf("%s %s -> home feed scroll error (non-critical): %v", tag, videoURL, err)
	}
	utils.Sleep(800, 2000)

	if _, err := page.Goto(videoURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   playwright.Float(30000),
	}); err != nil {
		log.Sugar().Infof("%s %s -> visit failed: %v", tag, videoURL, err)
		return models.VideoItem{}, false
	}
	utils.Sleep(3000, 5000)

	// Scroll a little to simulate watching / trigger any lazy-loaded stats.
	if err := utils.HumanScroll(page, utils.RandInt(1, 3)); err != nil {
		log.Sugar().Warnf("%s %s -> video page scroll error (non-critical): %v", tag, videoURL, err)
	}
	utils.Sleep(1500, 3000)

	mu.Lock()
	item := itemStruct
	mu.Unlock()

	if item == nil {
		item = extractFromRehydrationScript(page, log, tag, videoURL)
	}
	if item == nil {
		log.Sugar().Warnf("%s %s -> no video data extracted", tag, videoURL)
		return models.VideoItem{}, false
	}

	if raw, err := json.Marshal(item); err != nil {
		log.Sugar().Warnf("%s %s -> failed to marshal raw item for logging: %v", tag, videoURL, err)
	} else {
		log.Info("raw item data",
			zap.String("tag", tag),
			zap.String("url", videoURL),
			zap.String("raw_json", string(raw)),
		)
	}

	return parseItemStruct(v.cfg.OrgID, item), true
}

// extractFromRehydrationScript reads TikTok's embedded page-hydration JSON
// (script tag #__UNIVERSAL_DATA_FOR_REHYDRATION__) and pulls out the same
// itemStruct shape the item_detail XHR returns.
func extractFromRehydrationScript(page playwright.Page, log *zap.Logger, tag, videoURL string) map[string]any {
	raw, err := page.Evaluate(fmt.Sprintf(`() => {
		const el = document.getElementById(%q);
		return el ? el.textContent : null;
	}`, rehydrationScriptID))
	if err != nil {
		log.Sugar().Warnf("%s %s -> rehydration script read failed: %v", tag, videoURL, err)
		return nil
	}
	text, ok := raw.(string)
	if !ok || text == "" {
		return nil
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		log.Sugar().Warnf("%s %s -> rehydration JSON parse failed: %v", tag, videoURL, err)
		return nil
	}

	scope, _ := parsed["__DEFAULT_SCOPE__"].(map[string]any)
	detail, _ := scope["webapp.video-detail"].(map[string]any)
	info, _ := detail["itemInfo"].(map[string]any)
	item, _ := info["itemStruct"].(map[string]any)
	return item
}

// parseItemStruct maps a TikTok itemStruct (from either the item_detail XHR
// or the rehydration script) into a VideoItem. No keyword/cutoff filtering
// applies here — every URL was already selected by the caller (tbl_posts
// lookup), so every successfully-scraped item is returned.
func parseItemStruct(orgID int, item map[string]any) models.VideoItem {
	author, _ := item["author"].(map[string]any)
	stats, _ := item["stats"].(map[string]any)

	return models.VideoItem{
		OrgID:       orgID,
		VideoID:     toString(item["id"]),
		Description: toString(item["desc"]),
		PubTime:     flexInt64(item["createTime"]),
		UniqueID:    toString(mapGet(author, "uniqueId")),
		AuthID:      toString(mapGet(author, "id")),
		AuthName:    toString(mapGet(author, "nickname")),
		Comments:    flexInt64(mapGet(stats, "commentCount")),
		Shares:      flexInt64(mapGet(stats, "shareCount")),
		Reactions:   flexInt64(mapGet(stats, "diggCount")),
		Favors:      flexInt64(mapGet(stats, "collectCount")),
		Views:       flexInt64(mapGet(stats, "playCount")),
	}
}

// flexInt64 converts a JSON number or a numeric string into an int64. TikTok's
// embedded page-hydration JSON serializes large counters as strings (to avoid
// JS float precision loss) while the item_detail XHR returns plain numbers —
// this handles both.
func flexInt64(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case string:
		if i, err := strconv.ParseInt(n, 10, 64); err == nil {
			return i
		}
		if f, err := strconv.ParseFloat(n, 64); err == nil {
			return int64(f)
		}
	}
	return 0
}
