package repo

import (
	"context"
	"fmt"
	"fraud-detection-2026/internal/models"

	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"
)

type referenceRepo struct {
	db *pgxpool.Pool
}

func NewReferenceRepo(db *pgxpool.Pool) *referenceRepo {
	return &referenceRepo{db: db}
}

func (r *referenceRepo) FindSimilarTransactions(ctx context.Context, embedding []float32) ([]models.ReferenceVector, error) {
	const query = `
				SELECT
					id,
					is_fraud,
					vector <-> $1::vector AS distance
				FROM reference_vectors
				ORDER BY distance
				LIMIT 5;
				`
	rows, err := r.db.Query(ctx, query, pgvector.NewVector(embedding))
	if err != nil {
		return nil, fmt.Errorf("query similar transactions: %w", err)
	}
	defer rows.Close()

	results := make([]models.ReferenceVector, 0, 5)
	for rows.Next() {
		var result models.ReferenceVector
		if err := rows.Scan(&result.ID, &result.IsFraud, &result.Distance); err != nil {
			return nil, fmt.Errorf("scan similar transaction: %w", err)
		}
		results = append(results, result)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate similar transactions: %w", err)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no reference vectors found")
	}

	return results, nil
}
