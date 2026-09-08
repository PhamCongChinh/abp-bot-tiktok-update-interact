package crawler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"abp-bot-tiktok/internal/models"
	"abp-bot-tiktok/internal/parser"
	"abp-bot-tiktok/internal/utils"
	"abp-bot-tiktok/pkg/api"

	"go.uber.org/zap"
)

const (
	publisherBufferSize    = 100
	publisherNumWorkers    = 3
	publisherBatchMax      = 10
	publisherFlushInterval = 5 * time.Second
)

// Publisher handles video parsing and async batch API push operations.
// A worker pool collects videos from a buffered channel and flushes them
// to the backend API in batches (up to 10 videos or every 5 seconds).
// This provides backpressure — when the channel is full, sends are dropped
// and logged, preventing unbounded memory growth when the API is slow.
type Publisher struct {
	apiClient api.APIClient
	log       *zap.Logger
	videoCh   chan models.VideoItem
	workerWg  sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
	shutdown  sync.Once
}

// NewPublisher creates a Publisher with a worker pool. Workers start
// immediately and run until Shutdown is called.
func NewPublisher(apiClient api.APIClient, log *zap.Logger) *Publisher {
	p := &Publisher{
		apiClient: apiClient,
		log:       log,
		videoCh:   make(chan models.VideoItem, publisherBufferSize),
	}
	p.ctx, p.cancel = context.WithCancel(context.Background())
	for i := 0; i < publisherNumWorkers; i++ {
		p.workerWg.Add(1)
		go p.worker()
	}
	return p
}

// PushBatch sends video items to the internal channel for async batch
// processing. When the channel buffer is full, videos are dropped and a
// warning is logged — this provides backpressure under API slowdown.
// PushBatch returns immediately; the actual API push happens in workers.
func (p *Publisher) PushBatch(ctx context.Context, videos []models.VideoItem) {
	for _, v := range videos {
		select {
		case p.videoCh <- v:
		default:
			p.log.Warn("pushBatch: channel full, dropping video (backpressure)",
				zap.String("video_id", v.VideoID),
				zap.String("keyword", v.Keyword),
			)
		}
	}
}

// PushToAPI synchronously converts and sends a batch of video items to the
// backend API. This is used internally by workers and remains available for
// direct synchronous calls in tests.
func (p *Publisher) PushToAPI(ctx context.Context, videos []models.VideoItem) error {
	if len(videos) == 0 {
		return nil
	}

	posts := make([]parser.TiktokPost, 0, len(videos))
	for _, v := range videos {
		posts = append(posts, parser.FromVideoItem(v))
	}

	// Retry once with backoff for transient API failures.
	err := utils.RetryWithBackoff(ctx, 2, func() error {
		return p.apiClient.PostUnclassifiedBatch(ctx, posts)
	})
	if err != nil {
		return fmt.Errorf("pushToAPI: %w", err)
	}
	return nil
}

// Shutdown gracefully stops all workers, drains any remaining videos in the
// channel, and flushes the final batch. Blocks until all workers finish.
// Safe to call multiple times — subsequent calls are no-ops.
func (p *Publisher) Shutdown() {
	p.shutdown.Do(func() {
		// Signal workers to stop accepting new videos.
		p.cancel()
		// Close the channel to unblock any workers waiting on it.
		close(p.videoCh)
		// Wait for all workers to exit after flushing their last batch.
		p.workerWg.Wait()
	})
}

// worker is a goroutine that collects videos from the channel and flushes
// them in batches (max publisherBatchMax) or after publisherFlushInterval.
func (p *Publisher) worker() {
	defer p.workerWg.Done()

	batch := make([]models.VideoItem, 0, publisherBatchMax)
	timer := time.NewTimer(publisherFlushInterval)
	defer timer.Stop()

	for {
		select {
		case <-p.ctx.Done():
			// Drain remaining videos in the channel before flushing.
			for {
				select {
				case v, ok := <-p.videoCh:
					if !ok {
						// Channel closed — flush final batch.
						if len(batch) > 0 {
							p.flushBatch(batch)
						}
						return
					}
					batch = append(batch, v)
					if len(batch) >= publisherBatchMax {
						p.flushBatch(batch)
						batch = batch[:0]
					}
				default:
					// Channel drained — flush what's left.
					if len(batch) > 0 {
						p.flushBatch(batch)
					}
					return
				}
			}

		case v, ok := <-p.videoCh:
			if !ok {
				// Channel closed — flush final batch.
				if len(batch) > 0 {
					p.flushBatch(batch)
				}
				return
			}
			batch = append(batch, v)
			if len(batch) >= publisherBatchMax {
				p.flushBatch(batch)
				batch = batch[:0]
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(publisherFlushInterval)
			}

		case <-timer.C:
			if len(batch) > 0 {
				p.flushBatch(batch)
				batch = batch[:0]
			}
			timer.Reset(publisherFlushInterval)
		}
	}
}

// flushBatch sends a batch of videos to the API. Errors are logged as
// warnings but do not stop the worker.
func (p *Publisher) flushBatch(batch []models.VideoItem) {
	if len(batch) == 0 {
		return
	}

	// Use a background context with timeout for the flush so it's not
	// tied to the worker's shutdown context.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := p.PushToAPI(ctx, batch); err != nil {
		p.log.Warn("publisher: batch flush failed",
			zap.Int("batch_size", len(batch)),
			zap.Error(err),
		)
	} else {
		p.log.Info("publisher: batch flushed",
			zap.Int("batch_size", len(batch)),
		)
	}
}
