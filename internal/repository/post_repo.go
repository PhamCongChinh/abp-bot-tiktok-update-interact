package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// tiktokSourceCode matches parser.crawlSourceCode — the crawl_source_code
// value tagged on every post this bot pushes to tbl_posts.
const tiktokSourceCode = "tt"

// PostStore abstracts read access to tbl_posts (owned by the backend, not by
// this bot — no migration is run against it here).
type PostStore interface {
	FindURLsByOrgAndPubTime(ctx context.Context, orgID int, pubTimeAfter int64) ([]string, error)
}

type PostRepository struct {
	pool *pgxpool.Pool
	log  *zap.Logger
}

func NewPostRepository(pool *pgxpool.Pool, log *zap.Logger) *PostRepository {
	return &PostRepository{
		pool: pool,
		log:  log,
	}
}

// FindURLsByOrgAndPubTime returns the URLs of TikTok posts already recorded in
// tbl_posts for orgID, published after pubTimeAfter (unix seconds). Equivalent
// to:
//
//	SELECT url FROM tbl_posts tp
//	 WHERE tp.org_id = $1 AND tp.crawl_source_code = 'tt' AND tp.pub_time > $2
func (r *PostRepository) FindURLsByOrgAndPubTime(ctx context.Context, orgID int, pubTimeAfter int64) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	rows, err := r.pool.Query(ctx, `
		SELECT tp.url
		  FROM tbl_posts tp
		 WHERE tp.org_id = $1
		   AND tp.crawl_source_code = $2
		   AND tp.pub_time > $3
	`, orgID, tiktokSourceCode, pubTimeAfter)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var urls []string
	for rows.Next() {
		var url string
		if err := rows.Scan(&url); err != nil {
			return nil, err
		}
		urls = append(urls, url)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return urls, nil
}
