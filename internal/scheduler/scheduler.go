package scheduler

import (
	"context"
	"sync"
	"time"

	"abp-bot-tiktok/internal/crawler"
	"abp-bot-tiktok/pkg/config"

	"go.uber.org/zap"
)

// waitIfNightHours blocks until 03:00 if current time is between 00:00 and 03:00.
// Returns early if ctx is cancelled.
func waitIfNightHours(ctx context.Context, log *zap.Logger) {
	now := time.Now()
	if now.Hour() >= 0 && now.Hour() < 3 {
		next3am := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
		wait := time.Until(next3am)
		log.Sugar().Infof("Night hours (%02d:%02d) — sleeping until 03:00 (%s)", now.Hour(), now.Minute(), next3am.Format("2006-01-02 15:04:05"))
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
		}
	}
}

type Scheduler struct {
	cfg      *config.Config
	log      *zap.Logger
	crawler  *crawler.Crawler
	stopCh   chan struct{}
	stopOnce sync.Once
}

func New(cfg *config.Config, log *zap.Logger, c *crawler.Crawler) *Scheduler {
	return &Scheduler{
		cfg:     cfg,
		log:     log,
		crawler: c,
		stopCh:  make(chan struct{}),
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	intervalMin := 30
	intervalMax := 45

	s.log.Info("Scheduler started with interval mode",
		zap.Int("interval_min_minutes", intervalMin),
		zap.Int("interval_max_minutes", intervalMax),
	)

	waitIfNightHours(ctx, s.log)
	s.log.Info("Running initial crawl on startup...")
	s.crawler.Run(ctx)

	go s.runInterval(ctx, intervalMin, intervalMax)
}

func (s *Scheduler) runInterval(ctx context.Context, minMinutes, maxMinutes int) {
	for {
		intervalMinutes := minMinutes + int(time.Now().UnixNano()%int64(maxMinutes-minMinutes+1))
		interval := time.Duration(intervalMinutes) * time.Minute

		s.log.Info("Waiting for next crawl cycle",
			zap.Int("minutes", intervalMinutes),
			zap.String("next_run_at", time.Now().Add(interval).Format("2006-01-02 15:04:05")),
		)

		timer := time.NewTimer(interval)

		select {
		case <-timer.C:
			waitIfNightHours(ctx, s.log)
			s.log.Info("Interval triggered - starting new crawl cycle")
			s.crawler.Run(ctx)
		case <-s.stopCh:
			timer.Stop()
			s.log.Info("Scheduler stopped")
			return
		case <-ctx.Done():
			timer.Stop()
			s.log.Warn("scheduler: context cancelled, stopping interval runner")
			return
		}
	}
}

func (s *Scheduler) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
		s.log.Info("Scheduler stopping...")
	})
}
