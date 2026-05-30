package normalizer

import (
	"math"

	"fraud-detection-2026/internal/config"
	"fraud-detection-2026/internal/models"
	"fraud-detection-2026/internal/repo"
)

// LimitValue clamps a numeric value to the range [0,1] relative to max.
// If value > max it returns 1, if value < 0 it returns 0, otherwise value/max.
func LimitValue(value, max float64) float32 {
	if max == 0 {
		if value > 0 {
			return 1
		}
		return 0
	}

	ratio := value / max
	// clamp ratio to [0,1]
	ratio = math.Max(0, math.Min(1, ratio))
	return float32(ratio)
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
	v := make([]float32, 14)

	// amount
	v[0] = LimitValue(payload.Transaction.Amount, config.Cfg.MAX_AMOUNT)

	// installments
	v[1] = LimitValue(float64(payload.Transaction.Installments), float64(config.Cfg.MAX_INSTALLMENTS))

	// amount vs avg (compare transaction amount against customer's avg)
	if payload.Customer.AvgAmount > 0 {
		v[2] = LimitValue(payload.Transaction.Amount/payload.Customer.AvgAmount, config.Cfg.AMOUNT_VS_AVG_RATIO)
	}

	// hour of day (0-23)
	v[3] = LimitValue(float64(payload.Transaction.RequestedAt.Hour()), 23)

	// day of week: map Monday=0 ... Sunday=6, then normalize over 6
	wd := payload.Transaction.RequestedAt.Weekday()
	mappedWeekday := float64((int(wd) + 6) % 7)
	v[4] = LimitValue(mappedWeekday, 6)

	// when last transaction is present
	if payload.LastTransaction != nil {
		minutes := payload.Transaction.RequestedAt.Sub(payload.LastTransaction.Timestamp).Minutes()
		v[5] = LimitValue(minutes, float64(config.Cfg.MAX_MINUTES))
		v[6] = LimitValue(payload.LastTransaction.KmFromCurrent, config.Cfg.MAX_KM)
	} else {
		// sentinel for no previous transaction
		v[5] = -1
		v[6] = -1
	}

	// km from home
	v[7] = LimitValue(payload.Terminal.KmFromHome, config.Cfg.MAX_KM)

	// tx count 24h
	v[8] = LimitValue(float64(payload.Customer.TxCount24h), float64(config.Cfg.MAX_TX_COUNT_24H))

	// is_online
	if payload.Terminal.IsOnline {
		v[9] = 1
	}

	if payload.Terminal.CardPresent {
		v[10] = 1
	}

	// unknown merchant (1 if unknown, 0 if known)
	v[11] = IsUnknownMerchant(payload.Customer.KnownMerchants, payload.Merchant.ID)

	// mcc_risk
	v[12] = repo.GetMCCRiskScore(payload.Merchant.MCC)

	// merchant avg amount
	v[13] = LimitValue(payload.Merchant.AvgAmount, config.Cfg.MAX_MERCHANT_AVG_AMOUNT)

	return v
}
