//go:build goexperiment.simd

package index

import (
	"math"
	"math/rand"
	"testing"
)

// TestAVX2MatchesScalar checks the AVX2 kernel agrees with the scalar oracle
// within float rounding tolerance (FMA + tree reduction round differently than
// the sequential scalar sum).
func TestAVX2MatchesScalar(t *testing.T) {
	rng := rand.New(rand.NewSource(123))
	const n = 257 // not a multiple of any lane width
	vecs := make([]int8, (n+guardVecs)*PaddedDim)
	for i := range vecs {
		vecs[i] = int8(rng.Intn(255) - 127)
	}
	var q [PaddedDim]float32
	for i := 0; i < Dim; i++ {
		q[i] = rng.Float32()*2 - 1
	}

	got := make([]float32, n)
	want := make([]float32, n)
	distBatchAVX2(&q, vecs, n, got)
	distBatchScalar(&q, vecs, n, want)

	for i := 0; i < n; i++ {
		if d := math.Abs(float64(got[i] - want[i])); d > 1e-3 {
			t.Fatalf("vec %d: avx2=%v scalar=%v diff=%g", i, got[i], want[i], d)
		}
	}
}
