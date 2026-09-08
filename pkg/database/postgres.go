package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// PostgresDB wraps a pooled PostgreSQL connection.
type PostgresDB struct {
	pool *pgxpool.Pool
	log  *zap.Logger
}

// NewPostgresDB connects to PostgreSQL using dsn and returns a pooled client.
// It fails fast if the pool cannot be configured or the database is unreachable.
func NewPostgresDB(ctx context.Context, dsn string, maxPoolSize, minPoolSize int32, log *zap.Logger) (*PostgresDB, error) {
	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres parse dsn: %w", err)
	}
	poolCfg.MaxConns = maxPoolSize
	poolCfg.MinConns = minPoolSize

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("postgres connect: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}

	log.Info("PostgreSQL connected",
		zap.Int32("maxPoolSize", maxPoolSize),
		zap.Int32("minPoolSize", minPoolSize),
	)

	return &PostgresDB{pool: pool, log: log}, nil
}

func (p *PostgresDB) Close() {
	p.pool.Close()
}

func (p *PostgresDB) Pool() *pgxpool.Pool {
	return p.pool
}
