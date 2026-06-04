package repo

import (
	"context"

	"fraud-detection-2026/internal/index"
)

// referenceRepo answers nearest-neighbour queries from the in-process int8 IVF
// index (mmap'd references.bin) that replaced the previous Postgres/pgvector
// backend. There is no network or per-request allocation on this path.
type referenceRepo struct {
	idx *index.Index
}

func NewReferenceRepo(idx *index.Index) *referenceRepo {
	return &referenceRepo{idx: idx}
}

// NeighborStats summarises the k nearest reference vectors: how many of them are
// flagged as fraud out of the total returned.
type NeighborStats struct {
	Fraud int
	Total int
}

// FindSimilarTransactions returns fraud statistics for the k nearest reference
// vectors to the given embedding. The context is accepted to preserve the
// interface; the search is a synchronous in-memory scan and does not block on it.
func (r *referenceRepo) FindSimilarTransactions(_ context.Context, embedding []float32, k int) (NeighborStats, error) {
	s := r.idx.Search(embedding, k)
	return NeighborStats{Fraud: s.Fraud, Total: s.Total}, nil
}
