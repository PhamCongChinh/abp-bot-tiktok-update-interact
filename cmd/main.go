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
	// Fetch URLs of posts already recorded for ORG_ID/PUB_TIME. This is the
	// bot's sole source of crawl targets (no more keyword search), so a
	// failure here is fatal.
	dbCtx, dbCancel := context.WithTimeout(ctx, 30*time.Second)
	defer dbCancel()

	pg, err := database.NewPostgresDB(dbCtx, cfg.PostgresDSN,
		int32(cfg.PostgresMaxPoolSize), int32(cfg.PostgresMinPoolSize), log)
	if err != nil {
		log.Fatal("Failed to connect PostgreSQL", zap.Error(err))
	}
	defer pg.Close()

	postRepo := repository.NewPostRepository(pg.Pool(), log)
	postURLs, err := postRepo.FindURLsByOrgAndPubTime(dbCtx, cfg.OrgID, cfg.PubTimeAfter)
	if err != nil {
		log.Fatal("Failed to query tbl_posts", zap.Error(err))
	}

	log.Info("tbl_posts lookup complete",
		zap.Int("orgID", cfg.OrgID),
		zap.Int64("pubTimeAfter", cfg.PubTimeAfter),
		zap.Int("count", len(postURLs)),
	)
	for i, url := range postURLs {
		log.Info("tbl_posts url", zap.Int("index", i+1), zap.String("url", url))
	}

	if len(postURLs) == 0 {
		log.Warn("No post URLs found, exiting")
		return
	}

	// Set URLs to config (will be reused for all crawl cycles, shuffled each cycle in Run())
	cfg.PostURLs = postURLs

	// Init crawler
	c := crawler.New(&cfg, log, nil)

	log.Info("Crawler initialized - will visit tbl_posts URLs every 1-1.5 hours")

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
