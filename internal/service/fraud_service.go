package service

import (
	"context"

	"fraud-detection-2026/internal/repo"

	"github.com/jackc/pgx/v5/pgxpool"
)

// fraudService provides methods to calculate fraud scores.
// Mainly returns if a embbed transactions is fraud or not, and the fraud score itself.
type fraudService struct {
	db *pgxpool.Pool
}

func NewFraudService(db *pgxpool.Pool) *fraudService {
	return &fraudService{db: db}
}

func (s *fraudService) ScoreTransaction(ctx context.Context, embedding []float32) (bool, float32, error) {
	referenceRepo := repo.NewReferenceRepo(s.db) // FIXME: this should be injected, not created here
	matches, err := referenceRepo.FindSimilarTransactions(ctx, embedding)
	if err != nil {
		return false, 0, err
	}

	var fraudCount float32
	for _, match := range matches {
		if match.IsFraud {
			fraudCount++
		}
	}

	fraudScore := fraudCount / float32(len(matches))
	approved := fraudScore < 0.6

	return approved, fraudScore, nil // FIXME: I should be a struct
}
