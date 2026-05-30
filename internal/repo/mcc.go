package repo

var MCC_RISK_SCORES = map[string]float32{
	"5411": 0.15,
	"5812": 0.30,
	"5912": 0.20,
	"5944": 0.45,
	"7801": 0.80,
	"7802": 0.75,
	"7995": 0.85,
	"4511": 0.35,
	"5311": 0.25,
	"5999": 0.50,
}

// GetMCCRiskScore returns the risk score associated with a given MCC code.
func GetMCCRiskScore(mcc string) float32 {
	if score, exists := MCC_RISK_SCORES[mcc]; exists {
		return score
	}
	return 0.5 // Default risk score for unknown MCCs
}
