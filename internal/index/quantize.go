package index

import "math"

// Quantize maps a single reference value in [-1, 1] to int8 [-127, 127].
// Values are clamped defensively even though the dataset is already normalized.
func Quantize(x float32) int8 {
	q := math.RoundToEven(float64(x) * QuantScale)
	if q > 127 {
		q = 127
	} else if q < -127 {
		q = -127
	}
	return int8(q)
}

// QuantizeVec quantizes the first Dim values of v into a PaddedDim int8 array,
// zero-padding the trailing lanes. v must have at least Dim elements.
func QuantizeVec(v []float32) [PaddedDim]int8 {
	var out [PaddedDim]int8
	for i := 0; i < Dim; i++ {
		out[i] = Quantize(v[i])
	}
	return out
}

// padQuery copies a Dim-length (or longer) query into a PaddedDim float32 array,
// zeroing the padding lanes so they contribute nothing to the distance.
func padQuery(v []float32) [PaddedDim]float32 {
	var out [PaddedDim]float32
	copy(out[:Dim], v[:Dim])
	return out
}
