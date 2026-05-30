package normalizer

import (
	"fraud-detection-2026/internal/config"
	"fraud-detection-2026/internal/models"
)

// If number > max, return 1, else return number / max
func LimitValue(value, max float64) float32 {
	if value > max {
		return 1
	}
	return float32(value / max)
}

func NormalizePayloadTransaction(payload models.GetFraudScore) []float32 {
	vector := make([]float32, 14)

	// amount
	vector[0] = LimitValue(payload.Transaction.Amount, config.Cfg.MAX_AMOUNT)
	// installments
	vector[1] = LimitValue(payload.Customer.AvgAmount, config.Cfg.MAX_AMOUNT)

	return vector
}
