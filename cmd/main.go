package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"abp-bot-tiktok/internal/crawler"
	"abp-bot-tiktok/internal/repository"
	"abp-bot-tiktok/internal/scheduler"
	"abp-bot-tiktok/pkg/config"
	"abp-bot-tiktok/pkg/database"
	"abp-bot-tiktok/pkg/logger"

	"go.uber.org/zap"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: config load failed: %v\n", err)
		os.Exit(1)
	}

	logFilePath := filepath.Join(cfg.OutputDir, "logs", "bot.log")
	log, err := logger.New(logger.Config{
		Level:      cfg.LogLevel,
		FilePath:   logFilePath,
		MaxSizeMB:  cfg.LogMaxSizeMB,
		MaxAgeDays: cfg.LogMaxAgeDays,
		MaxBackups: cfg.LogMaxBackups,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: logger init failed: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = log.Sync() }()

	log.Info("Starting abp-bot-tiktok...")
	log.Sugar().Infof("DEBUG=%v | BotName=%s", cfg.Debug, cfg.BotName)

	// Top-level context for graceful shutdown.
	// All subsystems (PostgreSQL, scheduler, crawler) share this context so
	// a single cancel propagates cleanly through the entire call chain.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Wire OS signals to context cancellation.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Sugar().Infof("Received signal %v — initiating graceful shutdown", sig)
		cancel()
	}()

	// --- PostgreSQL: tbl_posts lookup ------------------------------------------
	// Fetch URLs of posts already recorded for ORG_ID/PUB_TIME. Non-fatal on
	// failure — this lookup does not gate the crawl loop below.
	dbCtx, dbCancel := context.WithTimeout(ctx, 30*time.Second)
	defer dbCancel()

	pg, err := database.NewPostgresDB(dbCtx, cfg.PostgresDSN,
		int32(cfg.PostgresMaxPoolSize), int32(cfg.PostgresMinPoolSize), log)
	if err != nil {
		log.Error("Failed to connect PostgreSQL — skipping tbl_posts lookup", zap.Error(err))
	} else {
		defer pg.Close()

		postRepo := repository.NewPostRepository(pg.Pool(), log)
		postURLs, err := postRepo.FindURLsByOrgAndPubTime(dbCtx, cfg.OrgID, cfg.PubTimeAfter)
		if err != nil {
			log.Error("Failed to query tbl_posts", zap.Error(err))
		} else {
			log.Info("tbl_posts lookup complete",
				zap.Int("orgID", cfg.OrgID),
				zap.Int64("pubTimeAfter", cfg.PubTimeAfter),
				zap.Int("count", len(postURLs)),
			)
			for i, url := range postURLs {
				log.Info("tbl_posts url", zap.Int("index", i+1), zap.String("url", url))
			}
		}
	}

	// --- PostgreSQL keyword lookup (disabled for local dev) -------------------
	// keywordRepo := repository.NewKeywordRepository(pg.Pool(), log)
	//
	// // Load keywords from PostgreSQL
	// keywords, err := keywordRepo.FindActive()
	// if err != nil {
	// 	log.Fatal("Failed to load keywords from PostgreSQL", zap.Error(err))
	// }
	// ---------------------------------------------------------------------------

	// Load keywords from local JSON file instead of PostgreSQL (local dev/testing).
	// The JSON file only lists keyword strings with no org distinction, so
	// every keyword is tagged with a fixed org_id of 0.
	const keywordsFile = "configs/keywords.json"
	log.Info("Loading keywords from local JSON file", zap.String("file", keywordsFile))

	keywords, err := repository.LoadKeywordsFromFile(keywordsFile, 0)
	if err != nil {
		log.Fatal("Failed to load keywords from JSON file", zap.Error(err))
	}

	var keywordList []string
	keywordOrgMap := make(map[string]int)
	for _, kw := range keywords {
		keywordList = append(keywordList, kw.Keyword)
		keywordOrgMap[kw.Keyword] = kw.OrgID
	}

	if len(keywordList) == 0 {
		log.Warn("No keywords found, exiting")
		return
	}

	log.Info("Keywords loaded",
		zap.Int("total", len(keywordList)),
		zap.Strings("keywords", keywordList),
	)

	// Set keywords to config (will be reused for all crawl cycles, shuffled each cycle in Run())
	cfg.Keywords = keywordList
	cfg.KeywordOrgMap = keywordOrgMap

	// Init crawler
	c := crawler.New(&cfg, log, nil)

	log.Info("Crawler initialized - will crawl same keywords every 1-1.5 hours")

	// Run crawler with the top-level context.
	// If SIGTERM/SIGINT is received, ctx is cancelled and the entire
	// call chain (scheduler → crawler → GPM) drains gracefully.
	runCrawler(ctx, &cfg, log, c)

	log.Info("Shutdown complete")
}

func runCrawler(pctx context.Context, cfg *config.Config, log *zap.Logger, c *crawler.Crawler) {
	// Derive a child context from the parent so that cancelling pctx (via
	// SIGTERM/SIGINT) propagates cleanly through scheduler and crawler.
	ctx, cancel := context.WithCancel(pctx)
	defer cancel()

	if cfg.Debug {
		log.Info("DEBUG mode: running crawler immediately")
		c.Run(ctx)
		log.Info("Done.")
		return
	}

	s := scheduler.New(cfg, log, c)
	s.Start(ctx)
	defer s.Stop()

	// Block until the parent context is cancelled (signal received).
	<-ctx.Done()
	log.Info("Shutting down — closing all profiles...")
}
