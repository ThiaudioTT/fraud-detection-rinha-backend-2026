package database

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgxvec "github.com/pgvector/pgvector-go/pgx"
)

// Connect builds a tuned pgx connection pool from DATABASE_URL and returns it to
// the caller. The pool is the single dependency the rest of the app is wired
// from, so we deliberately avoid a package-level singleton: ownership (and the
// matching Close) stays with main.
func Connect(ctx context.Context) (*pgxpool.Pool, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return nil, fmt.Errorf("DATABASE_URL is not set")
	}

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}

	// Pool sizing. Under the Rinha resource limits the bottleneck is the number
	// of in-flight HNSW searches, which equals the pool size. Keep a warm pool
	// (MinConns == MaxConns) so we never pay TCP+TLS+auth setup latency on the
	// hot path during a load spike. Overridable via DB_MAX_CONNS.
	maxConns := int32(max(4, runtime.GOMAXPROCS(0)*4))
	if v := os.Getenv("DB_MAX_CONNS"); v != "" {
		if n, perr := strconv.Atoi(v); perr == nil && n > 0 {
			maxConns = int32(n)
		}
	}
	cfg.MaxConns = maxConns
	cfg.MinConns = maxConns
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 30 * time.Minute
	cfg.HealthCheckPeriod = time.Minute

	// Register pgvector's types once per physical connection so we can bind
	// []float32 directly as a vector parameter.
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return pgxvec.RegisterTypes(ctx, conn)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}

	return pool, nil
}
