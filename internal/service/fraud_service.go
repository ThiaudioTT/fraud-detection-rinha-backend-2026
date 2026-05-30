package service

import "github.com/jackc/pgx/v5/pgxpool"

// fraudService provides methods to calculate fraud scores.
// Mainly returns if a embbed transactions is fraud or not, and the fraud score itself.
type fraudService struct {
	db *pgxpool.Pool
}

func NewFraudService(db *pgxpool.Pool) *fraudService {
	return &fraudService{db: db}
}
