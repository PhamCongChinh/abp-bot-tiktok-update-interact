package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type BotConfig struct {
	ID        int64
	BotName   string
	BotType   string // "video" or "comment"
	OrgIDs    []string
	Sleep     int // minutes
	GPMAPI    string
	ProfileID string
	Active    bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

const createBotConfigTableSQL = `
CREATE TABLE IF NOT EXISTS tiktok_bot_configs (
	id SERIAL PRIMARY KEY,
	bot_name TEXT NOT NULL UNIQUE,
	bot_type TEXT NOT NULL,
	org_ids TEXT[] NOT NULL DEFAULT '{}',
	sleep INTEGER NOT NULL DEFAULT 0,
	gpm_api TEXT,
	profile_id TEXT,
	active BOOLEAN NOT NULL DEFAULT true,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL
);
`

type BotConfigRepository struct {
	pool *pgxpool.Pool
	log  *zap.Logger
}

func NewBotConfigRepository(pool *pgxpool.Pool, log *zap.Logger) *BotConfigRepository {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := pool.Exec(ctx, createBotConfigTableSQL); err != nil {
		log.Warn("Failed to ensure tiktok_bot_configs table (may already exist)", zap.Error(err))
	} else {
		log.Info("tiktok_bot_configs table ready")
	}

	return &BotConfigRepository{
		pool: pool,
		log:  log,
	}
}

func (r *BotConfigRepository) FindByBotName(botName string) (*BotConfig, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var config BotConfig
	err := r.pool.QueryRow(ctx,
		`SELECT id, bot_name, bot_type, org_ids, sleep, gpm_api, profile_id, active, created_at, updated_at
		   FROM tiktok_bot_configs WHERE bot_name = $1`, botName,
	).Scan(&config.ID, &config.BotName, &config.BotType, &config.OrgIDs, &config.Sleep,
		&config.GPMAPI, &config.ProfileID, &config.Active, &config.CreatedAt, &config.UpdatedAt)
	if err != nil {
		return nil, err
	}

	return &config, nil
}

func (r *BotConfigRepository) FindActive() ([]BotConfig, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := r.pool.Query(ctx,
		`SELECT id, bot_name, bot_type, org_ids, sleep, gpm_api, profile_id, active, created_at, updated_at
		   FROM tiktok_bot_configs WHERE active = true`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []BotConfig
	for rows.Next() {
		var c BotConfig
		if err := rows.Scan(&c.ID, &c.BotName, &c.BotType, &c.OrgIDs, &c.Sleep,
			&c.GPMAPI, &c.ProfileID, &c.Active, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		configs = append(configs, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return configs, nil
}

func (r *BotConfigRepository) Upsert(config *BotConfig) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	config.UpdatedAt = time.Now()
	if config.CreatedAt.IsZero() {
		config.CreatedAt = time.Now()
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO tiktok_bot_configs (bot_name, bot_type, org_ids, sleep, gpm_api, profile_id, active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (bot_name) DO UPDATE SET
			bot_type = EXCLUDED.bot_type,
			org_ids = EXCLUDED.org_ids,
			sleep = EXCLUDED.sleep,
			gpm_api = EXCLUDED.gpm_api,
			profile_id = EXCLUDED.profile_id,
			active = EXCLUDED.active,
			updated_at = EXCLUDED.updated_at
	`, config.BotName, config.BotType, config.OrgIDs, config.Sleep,
		config.GPMAPI, config.ProfileID, config.Active, config.CreatedAt, config.UpdatedAt)

	return err
}
