package service

import "github.com/jackc/pgx/v5/pgxpool"

// FraudService provides methods to calculate fraud scores.
// Mainly returns if a embbed transactions is fraud or not, and the fraud score itself.
type FraudService struct {
	db *pgxpool.Pool
}

func NewFraudService(db *pgxpool.Pool) *FraudService {
	return &FraudService{db: db}
}
