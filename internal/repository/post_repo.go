package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// tiktokSourceCode matches parser.crawlSourceCode — the crawl_source_code
// value tagged on every post this bot pushes to tbl_posts.
const tiktokSourceCode = "tt"

// PostStore abstracts access to tbl_posts (owned by the backend, not by this
// bot — no migration is run against it here).
type PostStore interface {
	FindURLsByOrgAndPubTime(ctx context.Context, orgID int, pubTimeAfter int64) ([]string, error)
	UpdateStatsByURL(ctx context.Context, url string, comments, shares, reactions, favors, views int64) (int64, error)
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

// UpdateStatsByURL refreshes the engagement stats of a tbl_posts row after a
// re-visit and returns the number of rows affected (0 means no post in
// tbl_posts matched the given URL). Equivalent to:
//
//	UPDATE tbl_posts tp
//	   SET comments = $1, shares = $2, reactions = $3, favors = $4, views = $5
//	 WHERE tp.url = $6
func (r *PostRepository) UpdateStatsByURL(ctx context.Context, url string, comments, shares, reactions, favors, views int64) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	tag, err := r.pool.Exec(ctx, `
		UPDATE tbl_posts tp
		   SET comments = $1,
		       shares = $2,
		       reactions = $3,
		       favors = $4,
		       views = $5
		 WHERE tp.url = $6
	`, comments, shares, reactions, favors, views, url)
	if err != nil {
		return 0, fmt.Errorf("UpdateStatsByURL %q: %w", url, err)
	}

	rowsAffected := tag.RowsAffected()
	if rowsAffected == 0 {
		r.log.Warn("UpdateStatsByURL: no matching row in tbl_posts", zap.String("url", url))
	}

	return rowsAffected, nil
}
