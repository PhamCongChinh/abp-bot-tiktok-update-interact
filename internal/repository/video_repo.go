package repository

import (
	"context"
	"fmt"
	"time"

	"abp-bot-tiktok/internal/models"
	"abp-bot-tiktok/internal/utils"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// VideoStore abstracts video persistence operations.
// Define at consumer side — Crawler depends on this, not on *VideoRepository.
type VideoStore interface {
	FindByKeyword(ctx context.Context, keyword string, limit int64) ([]VideoDocument, error)
	UpsertVideo(ctx context.Context, video models.VideoItem) error
	BulkUpsert(ctx context.Context, videos []models.VideoItem) error
}

type VideoDocument struct {
	Keyword     string
	VideoID     string
	Description string
	PubTime     int64
	UniqueID    string
	AuthID      string
	AuthName    string
	Comments    int64
	Shares      int64
	Reactions   int64
	Favors      int64
	Views       int64
	CrawledAt   time.Time
	UpdatedAt   time.Time
}

const createVideoTableSQL = `
CREATE TABLE IF NOT EXISTS tiktok_videos (
	id SERIAL PRIMARY KEY,
	keyword TEXT NOT NULL,
	video_id TEXT NOT NULL UNIQUE,
	description TEXT,
	pub_time BIGINT,
	unique_id TEXT,
	auth_id TEXT,
	auth_name TEXT,
	comments BIGINT,
	shares BIGINT,
	reactions BIGINT,
	favors BIGINT,
	views BIGINT,
	crawled_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_tiktok_videos_keyword ON tiktok_videos(keyword);
`

type VideoRepository struct {
	pool *pgxpool.Pool
	log  *zap.Logger
}

func NewVideoRepository(pool *pgxpool.Pool, log *zap.Logger) *VideoRepository {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := pool.Exec(ctx, createVideoTableSQL); err != nil {
		log.Warn("Failed to ensure tiktok_videos table (may already exist)", zap.Error(err))
	} else {
		log.Info("tiktok_videos table ready")
	}

	return &VideoRepository{
		pool: pool,
		log:  log,
	}
}

func (r *VideoRepository) Upsert(ctx context.Context, video models.VideoItem) error {
	return r.UpsertVideo(ctx, video)
}

// UpsertVideo inserts or updates a single video row. Satisfies the
// VideoStore interface.
func (r *VideoRepository) UpsertVideo(ctx context.Context, video models.VideoItem) error {
	now := time.Now()

	err := utils.RetryWithBackoff(ctx, 3, func() error {
		_, err := r.pool.Exec(ctx, upsertVideoSQL,
			video.Keyword, video.VideoID, video.Description, video.PubTime,
			video.UniqueID, video.AuthID, video.AuthName,
			video.Comments, video.Shares, video.Reactions, video.Favors, video.Views,
			now, now,
		)
		return err
	})
	if err != nil {
		return fmt.Errorf("Upsert video %s: %w", video.VideoID, err)
	}
	return nil
}

const upsertVideoSQL = `
INSERT INTO tiktok_videos (
	keyword, video_id, description, pub_time, unique_id, auth_id, auth_name,
	comments, shares, reactions, favors, views, crawled_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
ON CONFLICT (video_id) DO UPDATE SET
	keyword = EXCLUDED.keyword,
	description = EXCLUDED.description,
	pub_time = EXCLUDED.pub_time,
	unique_id = EXCLUDED.unique_id,
	auth_id = EXCLUDED.auth_id,
	auth_name = EXCLUDED.auth_name,
	comments = EXCLUDED.comments,
	shares = EXCLUDED.shares,
	reactions = EXCLUDED.reactions,
	favors = EXCLUDED.favors,
	views = EXCLUDED.views,
	updated_at = EXCLUDED.updated_at
`

func (r *VideoRepository) BulkUpsert(ctx context.Context, videos []models.VideoItem) error {
	if len(videos) == 0 {
		return nil
	}

	now := time.Now()

	err := utils.RetryWithBackoff(ctx, 3, func() error {
		batch := &pgx.Batch{}
		for _, video := range videos {
			batch.Queue(upsertVideoSQL,
				video.Keyword, video.VideoID, video.Description, video.PubTime,
				video.UniqueID, video.AuthID, video.AuthName,
				video.Comments, video.Shares, video.Reactions, video.Favors, video.Views,
				now, now,
			)
		}

		br := r.pool.SendBatch(ctx, batch)
		defer func() { _ = br.Close() }()

		for i := 0; i < batch.Len(); i++ {
			if _, execErr := br.Exec(); execErr != nil {
				return execErr
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("BulkUpsert %d videos: %w", len(videos), err)
	}

	r.log.Info("Bulk upsert completed", zap.Int("count", len(videos)))

	return nil
}

func (r *VideoRepository) FindByKeyword(ctx context.Context, keyword string, limit int64) ([]VideoDocument, error) {
	var rows []VideoDocument

	err := utils.RetryWithBackoff(ctx, 3, func() error {
		pgxRows, err := r.pool.Query(ctx,
			`SELECT keyword, video_id, description, pub_time, unique_id, auth_id, auth_name,
			        comments, shares, reactions, favors, views, crawled_at, updated_at
			   FROM tiktok_videos
			  WHERE keyword = $1
			  ORDER BY pub_time DESC
			  LIMIT $2`, keyword, limit)
		if err != nil {
			return err
		}
		defer pgxRows.Close()

		rows = nil
		for pgxRows.Next() {
			var v VideoDocument
			if scanErr := pgxRows.Scan(
				&v.Keyword, &v.VideoID, &v.Description, &v.PubTime,
				&v.UniqueID, &v.AuthID, &v.AuthName,
				&v.Comments, &v.Shares, &v.Reactions, &v.Favors, &v.Views,
				&v.CrawledAt, &v.UpdatedAt,
			); scanErr != nil {
				return scanErr
			}
			rows = append(rows, v)
		}
		return pgxRows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("FindByKeyword %q: %w", keyword, err)
	}

	return rows, nil
}
