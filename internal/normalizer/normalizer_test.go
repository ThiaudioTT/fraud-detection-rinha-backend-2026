package normalizer

import (
	"testing"
	"time"

	"fraud-detection-2026/internal/models"
)

func approxEq(a, b float32) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 1e-4
}

func TestNormalizeDimensionsAndUTC(t *testing.T) {
	// 22:30 at -03:00 is 01:30 UTC the next day (2026-06-04, a Thursday).
	// Thursday maps to 3 in the mon=0..sun=6 scheme.
	loc := time.FixedZone("BRT", -3*3600)
	reqAt := time.Date(2026, 6, 3, 22, 30, 0, 0, loc)

	payload := models.FraudScoreRequest{
		Transaction: models.Transaction{Amount: 1000, Installments: 6, RequestedAt: reqAt},
		Customer:    models.Customer{AvgAmount: 500, TxCount24h: 10, KnownMerchants: []string{"m1"}},
		Merchant:    models.Merchant{ID: "m9", MCC: "7995", AvgAmount: 2000},
		Terminal:    models.Terminal{IsOnline: true, CardPresent: false, KmFromHome: 500},
		LastTransaction: nil,
	}

	v := NormalizePayloadTransaction(payload)
	if len(v) != 14 {
		t.Fatalf("want 14 dims, got %d", len(v))
	}

	checks := map[int]float32{
		0:  0.1,        // 1000/10000
		1:  0.5,        // 6/12
		2:  0.2,        // (1000/500)/10 = 0.2
		3:  1.0 / 23.0, // UTC hour = 1 (NOT 22)
		4:  0.5,        // Thursday=3 -> 3/6
		5:  -1,         // no last_transaction
		6:  -1,
		7:  0.5,  // 500/1000 km
		8:  0.5,  // 10/20
		9:  1,    // is_online
		10: 0,    // card_present false
		11: 1,    // unknown merchant (m9 not in [m1])
		12: 0.85, // mcc 7995
		13: 0.2,  // 2000/10000
	}
	for i, want := range checks {
		if !approxEq(v[i], want) {
			t.Errorf("dim %d: got %v want %v", i, v[i], want)
		}
	}
}

func TestKnownMerchant(t *testing.T) {
	p := models.FraudScoreRequest{
		Merchant: models.Merchant{ID: "m1"},
		Customer: models.Customer{KnownMerchants: []string{"m1", "m2"}},
	}
	if v := NormalizePayloadTransaction(p); v[11] != 0 {
		t.Errorf("known merchant should encode 0, got %v", v[11])
	}
}
