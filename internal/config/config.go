package config

type Config struct {
	MAX_AMOUNT              float64
	MAX_INSTALLMENTS        int
	AMOUNT_VS_AVG_RATIO     float64
	MAX_MINUTES             int
	MAX_KM                  float64
	MAX_TX_COUNT_24H        int
	MAX_MERCHANT_AVG_AMOUNT float64
	DATABASE_URL            string
}

func load() Config {
	return Config{
		// We hardcoding for simplicity! In real prod, keep the envs ;)
		MAX_AMOUNT:              10000,
		MAX_INSTALLMENTS:        12,
		AMOUNT_VS_AVG_RATIO:     10,
		MAX_MINUTES:             1440,
		MAX_KM:                  1000,
		MAX_TX_COUNT_24H:        20,
		MAX_MERCHANT_AVG_AMOUNT: 10000,
		DATABASE_URL:            "postgres://user:password@localhost:5432/dbname",
	}
}

var Cfg = load() // Global config instance
