package index

// distBatch computes the asymmetric squared-L2 distance from the padded float
// query q to each of n consecutive PaddedDim-wide int8 vectors in vecs, writing
// the results into out[:n].
//
// "Asymmetric" means only the reference side is quantized: each int8 lane is
// dequantized to float (lane * invScale) and compared against the full-precision
// query, so the query contributes no quantization error. Squared distance is
// sufficient because we only ever rank by it.
//
// It is a package-level variable so an AVX2 implementation can replace it at
// init on capable CPUs (see distance_amd64.go); the scalar version below is the
// portable fallback and the correctness oracle for tests.
var distBatch = distBatchScalar

func distBatchScalar(q *[PaddedDim]float32, vecs []int8, n int, out []float32) {
	for j := 0; j < n; j++ {
		base := j * PaddedDim
		v := vecs[base : base+PaddedDim : base+PaddedDim]
		var sum float32
		for i := 0; i < PaddedDim; i++ {
			d := float32(v[i])*invScale - q[i]
			sum += d * d
		}
		out[j] = sum
	}
}

// centDistBatch computes the exact squared-L2 distance from q to each of n
// consecutive PaddedDim float32 centroids, writing them to out[:n]. This is the
// fixed per-query cost (NClusters distances) and dominates at low nprobe, so it
// gets the same scalar/AVX2 treatment as the candidate kernel.
var centDistBatch = centDistBatchScalar

func centDistBatchScalar(q *[PaddedDim]float32, cents []float32, n int, out []float32) {
	for j := 0; j < n; j++ {
		base := j * PaddedDim
		c := cents[base : base+PaddedDim : base+PaddedDim]
		var sum float32
		for i := 0; i < PaddedDim; i++ {
			d := c[i] - q[i]
			sum += d * d
		}
		out[j] = sum
	}
}
