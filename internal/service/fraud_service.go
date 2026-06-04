package service

import (
	"context"
	"fmt"

	"fraud-detection-2026/internal/repo"
)

// neighborCount is how many nearest reference vectors we score a transaction
// against, and fraudThreshold is the share of fraudulent neighbors at (or above)
// which a transaction is rejected.
const (
	neighborCount  = 5
	fraudThreshold = 0.6
)

// ReferenceRepository retrieves fraud statistics for the nearest reference
// vectors. Declaring the dependency as an interface here (where it is consumed)
// keeps the service decoupled from pgx and trivially mockable in tests.
type ReferenceRepository interface {
	FindSimilarTransactions(ctx context.Context, embedding []float32, k int) (repo.NeighborStats, error)
}

// FraudResult is the outcome of scoring a single transaction.
type FraudResult struct {
	Approved   bool    `json:"approved"`
	FraudScore float32 `json:"fraud_score"`
}

// FraudService decides whether an embedded transaction is fraudulent based on
// the labels of its nearest reference vectors.
type FraudService struct {
	repo ReferenceRepository
}

func NewFraudService(repo ReferenceRepository) *FraudService {
	return &FraudService{repo: repo}
}

func (s *FraudService) ScoreTransaction(ctx context.Context, embedding []float32) (FraudResult, error) {
	stats, err := s.repo.FindSimilarTransactions(ctx, embedding, neighborCount)
	if err != nil {
		return FraudResult{}, err
	}
	if stats.Total == 0 {
		return FraudResult{}, fmt.Errorf("no reference vectors found")
	}

	fraudScore := float32(stats.Fraud) / float32(stats.Total)
	return FraudResult{
		Approved:   fraudScore < fraudThreshold,
		FraudScore: fraudScore,
	}, nil
}
