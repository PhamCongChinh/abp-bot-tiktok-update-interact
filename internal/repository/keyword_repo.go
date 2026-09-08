package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// KeywordStore abstracts keyword persistence operations.
// Define at consumer side so main() and Crawler depend on this, not on
// *KeywordRepository.
type KeywordStore interface {
	FindByOrgIDs(orgIDs []int) ([]Keyword, error)
	FindActive() ([]Keyword, error)
}

type Keyword struct {
	ID      int64
	Keyword string
	OrgID   int
	Active  bool
}

// LoadKeywordsFromFile reads a plain keyword list from a local JSON file, used
// as a PostgreSQL-free alternative to FindByOrgIDs for local development/testing
// (see cmd/main.go). The file must contain a JSON array of strings, e.g.
// configs/keywords.json. Every keyword is tagged with defaultOrgID since the
// file carries no per-keyword org mapping.
func LoadKeywordsFromFile(path string, defaultOrgID int) ([]Keyword, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read keywords file %s: %w", path, err)
	}

	var raw []string
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse keywords file %s: %w", path, err)
	}

	keywords := make([]Keyword, 0, len(raw))
	for _, kw := range raw {
		keywords = append(keywords, Keyword{Keyword: kw, OrgID: defaultOrgID, Active: true})
	}

	return keywords, nil
}

const createKeywordTableSQL = `
CREATE TABLE IF NOT EXISTS keyword (
	id SERIAL PRIMARY KEY,
	keyword TEXT NOT NULL,
	org_id INTEGER NOT NULL,
	active BOOLEAN NOT NULL DEFAULT true
);
CREATE INDEX IF NOT EXISTS idx_keyword_org_id ON keyword(org_id);
CREATE INDEX IF NOT EXISTS idx_keyword_active ON keyword(active);
`

type KeywordRepository struct {
	pool *pgxpool.Pool
	log  *zap.Logger
}

func NewKeywordRepository(pool *pgxpool.Pool, log *zap.Logger) *KeywordRepository {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := pool.Exec(ctx, createKeywordTableSQL); err != nil {
		log.Warn("Failed to ensure keyword table (may already exist)", zap.Error(err))
	} else {
		log.Info("keyword table ready")
	}

	return &KeywordRepository{
		pool: pool,
		log:  log,
	}
}

func (r *KeywordRepository) FindByOrgIDs(orgIDs []int) ([]Keyword, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := r.pool.Query(ctx,
		`SELECT id, keyword, org_id, active FROM keyword WHERE org_id = ANY($1)`, orgIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanKeywords(rows)
}

func (r *KeywordRepository) FindActive() ([]Keyword, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := r.pool.Query(ctx,
		`SELECT id, keyword, org_id, active FROM keyword WHERE active = true`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanKeywords(rows)
}

func scanKeywords(rows pgx.Rows) ([]Keyword, error) {
	var keywords []Keyword
	for rows.Next() {
		var k Keyword
		if err := rows.Scan(&k.ID, &k.Keyword, &k.OrgID, &k.Active); err != nil {
			return nil, err
		}
		keywords = append(keywords, k)
	}
	return keywords, rows.Err()
}
