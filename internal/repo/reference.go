package repo

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"
)

type referenceRepo struct {
	db *pgxpool.Pool
}

func NewReferenceRepo(db *pgxpool.Pool) *referenceRepo {
	return &referenceRepo{db: db}
}

// NeighborStats summarises the k nearest reference vectors: how many of them are
// flagged as fraud out of the total returned.
type NeighborStats struct {
	Fraud int
	Total int
}

// FindSimilarTransactions returns fraud statistics for the k nearest reference
// vectors to the given embedding.
//
// We let Postgres do the counting in a single round trip instead of streaming k
// rows back and looping in Go: the inner query uses the HNSW index for the
// ORDER BY ... LIMIT (selecting only is_fraud, no unused columns), and the outer
// aggregate collapses the result to two integers. This is the hot path, so it
// runs as a cached prepared statement on a warm pooled connection.
func (r *referenceRepo) FindSimilarTransactions(ctx context.Context, embedding []float32, k int) (NeighborStats, error) {
	const query = `
		SELECT
			count(*) FILTER (WHERE is_fraud)::int AS fraud,
			count(*)::int                          AS total
		FROM (
			SELECT is_fraud
			FROM reference_vectors
			ORDER BY vector <-> $1::vector
			LIMIT $2
		) AS neighbors`

	var stats NeighborStats
	err := r.db.QueryRow(ctx, query, pgvector.NewVector(embedding), k).
		Scan(&stats.Fraud, &stats.Total)
	if err != nil {
		return NeighborStats{}, fmt.Errorf("score nearest reference vectors: %w", err)
	}

	return stats, nil
}
