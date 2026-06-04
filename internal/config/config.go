package config

// Config holds the normalization constants from the challenge's
// normalization.json. The values below match that file exactly (verified); the
// embedding must use these so query vectors land in the same space as the
// pre-embedded reference dataset.
type Config struct {
	MAX_AMOUNT              float64
	MAX_INSTALLMENTS        int
	AMOUNT_VS_AVG_RATIO     float64
	MAX_MINUTES             int
	MAX_KM                  float64
	MAX_TX_COUNT_24H        int
	MAX_MERCHANT_AVG_AMOUNT float64
}

func load() Config {
	return Config{
		MAX_AMOUNT:              10000,
		MAX_INSTALLMENTS:        12,
		AMOUNT_VS_AVG_RATIO:     10,
		MAX_MINUTES:             1440,
		MAX_KM:                  1000,
		MAX_TX_COUNT_24H:        20,
		MAX_MERCHANT_AVG_AMOUNT: 10000,
	}
}

var Cfg = load() // Global config instance
