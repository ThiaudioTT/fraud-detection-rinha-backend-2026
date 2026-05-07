package database

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgxvec "github.com/pgvector/pgvector-go/pgx"
)

var Pool *pgxpool.Pool

func Connect(ctx context.Context) error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL is not set")
	}

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("parse DATABASE_URL: %w", err)
	}

	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return pgxvec.RegisterTypes(ctx, conn)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("create pgx pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return fmt.Errorf("ping db: %w", err)
	}

	Pool = pool
	return nil
}

func Close() {
	if Pool != nil {
		Pool.Close()
		Pool = nil
	}
}
