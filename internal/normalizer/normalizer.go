package normalizer

import (
	"fraud-detection-2026/internal/config"
	"fraud-detection-2026/internal/models"
	"fraud-detection-2026/internal/repo"
)

// If number > max, return 1, else return number / max
func LimitValue(value, max float64) float32 {
	if value > max {
		return 1
	}
	return float32(value / max)
}

// 1 se merchant.id não estiver em customer.known_merchants, senão 0 (invertido: 1 = desconhecido)
func IsUnknownMerchant(knownMerchants []string, merchantID string) float32 {
	for _, m := range knownMerchants {
		if m == merchantID {
			return 0
		}
	}
	return 1
}

// Return the embedded vector of the transaction
func NormalizePayloadTransaction(payload models.FraudScoreRequest) []float32 {
	vector := make([]float32, 14)

	// amount
	vector[0] = LimitValue(payload.Transaction.Amount/config.Cfg.MAX_AMOUNT, config.Cfg.MAX_AMOUNT)
	// installments
	vector[1] = LimitValue(float64(payload.Transaction.Installments)/float64(config.Cfg.MAX_INSTALLMENTS), float64(config.Cfg.MAX_INSTALLMENTS))
	// amount vs avg
	vector[2] = LimitValue((payload.Transaction.Amount/payload.Customer.AvgAmount)/config.Cfg.AMOUNT_VS_AVG_RATIO, config.Cfg.AMOUNT_VS_AVG_RATIO)
	// hour of day
	vector[3] = LimitValue(float64(payload.Transaction.RequestedAt.Hour())/23, 1)

	// day_of_week
	vector[4] = LimitValue(float64(payload.Transaction.RequestedAt.Weekday())/6, 1)

	// when last transaction is not null
	if payload.LastTransaction != nil {
		// minutes since last transaction
		vector[5] = LimitValue(float64(payload.Transaction.RequestedAt.Sub(payload.LastTransaction.Timestamp).Minutes())/float64(config.Cfg.MAX_MINUTES), 1)
		// km from last transaction
		vector[6] = LimitValue(payload.LastTransaction.KmFromCurrent/config.Cfg.MAX_KM, 1)
	} else {
		// If no last transaction, we put -1
		vector[5] = -1
		vector[6] = -1
	}

	// km from home
	vector[7] = LimitValue(payload.Terminal.KmFromHome/config.Cfg.MAX_KM, 1)

	// tx count 24h
	vector[8] = LimitValue(float64(payload.Customer.TxCount24h)/float64(config.Cfg.MAX_TX_COUNT_24H), 1)

	// is_online
	if payload.Terminal.IsOnline {
		vector[9] = 1
	}

	if payload.Terminal.CardPresent {
		vector[10] = 1
	}

	// unknown merchant (1 if unknown, 0 if known)
	vector[11] = IsUnknownMerchant(payload.Customer.KnownMerchants, payload.Merchant.ID)

	// mcc_risk
	vector[12] = repo.GetMCCRiskScore(payload.Merchant.MCC)

	// merchant avg amount
	vector[13] = LimitValue(payload.Merchant.AvgAmount/config.Cfg.MAX_MERCHANT_AVG_AMOUNT, 1)

	return vector
}
