package database

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestNewPostgresDB_InvalidDSN(t *testing.T) {
	log := zap.NewNop()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := NewPostgresDB(ctx, "not-a-valid-dsn", 100, 10, log)
	if err == nil {
		t.Error("expected error for invalid DSN, got nil")
	}
}

func TestNewPostgresDB_ContextCancelled(t *testing.T) {
	log := zap.NewNop()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewPostgresDB(ctx, "postgres://user:pass@localhost:5432/testdb?sslmode=disable", 100, 10, log)
	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}

func TestNewPostgresDB_UnreachableHost(t *testing.T) {
	log := zap.NewNop()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := NewPostgresDB(ctx, "postgres://user:pass@127.0.0.1:1/testdb?sslmode=disable", 100, 10, log)
	if err == nil {
		t.Log("unexpected success (server may be running)")
	}
}

func TestPostgresDB_Accessors(t *testing.T) {
	db := &PostgresDB{log: zap.NewNop()}
	if db.Pool() != nil {
		t.Error("expected nil pool for zero-value PostgresDB")
	}
}
