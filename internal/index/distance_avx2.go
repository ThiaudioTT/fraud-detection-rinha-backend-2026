//go:build goexperiment.simd

package index

import (
	"simd/archsimd"
	"unsafe"
)

// init swaps in the AVX2 distance kernel when the CPU supports it. Built only
// with GOEXPERIMENT=simd; otherwise distBatch stays the scalar version.
func init() {
	if archsimd.X86.AVX2() {
		distBatch = distBatchAVX2
		centDistBatch = centDistBatchAVX2
	}
}

// centDistBatchAVX2 is the AVX2 float32 L2 kernel for centroid selection. Each
// centroid is a full PaddedDim float32 (no quantization), so it loads cleanly in
// two 8-lane halves with no guard needed.
func centDistBatchAVX2(q *[PaddedDim]float32, cents []float32, n int, out []float32) {
	q0 := archsimd.LoadFloat32x8((*[8]float32)(unsafe.Pointer(&q[0])))
	q1 := archsimd.LoadFloat32x8((*[8]float32)(unsafe.Pointer(&q[8])))
	for j := 0; j < n; j++ {
		base := j * PaddedDim
		c0 := archsimd.LoadFloat32x8((*[8]float32)(unsafe.Pointer(&cents[base])))
		c1 := archsimd.LoadFloat32x8((*[8]float32)(unsafe.Pointer(&cents[base+8])))
		d0 := c0.Sub(q0)
		d1 := c1.Sub(q1)
		acc := d0.Mul(d0)
		acc = d1.MulAdd(d1, acc)
		out[j] = hsum8(acc)
	}
}

// distBatchAVX2 is the AVX2 implementation of the asymmetric squared-L2 kernel.
// Each PaddedDim (16) reference vector is loaded as int8, widened to float, and
// compared against the full-precision query in two 8-lane halves. The two halves
// are accumulated with an FMA and reduced to a scalar per vector.
//
// The 16-int8 load reads from &vecs[base+8] for the high half; the format
// reserves guardVecs trailing zero rows so this never reads past the mapping.
func distBatchAVX2(q *[PaddedDim]float32, vecs []int8, n int, out []float32) {
	invS := archsimd.BroadcastFloat32x8(invScale)
	q0 := archsimd.LoadFloat32x8((*[8]float32)(unsafe.Pointer(&q[0])))
	q1 := archsimd.LoadFloat32x8((*[8]float32)(unsafe.Pointer(&q[8])))

	for j := 0; j < n; j++ {
		base := j * PaddedDim
		lo := archsimd.LoadInt8x16((*[16]int8)(unsafe.Pointer(&vecs[base])))
		hi := archsimd.LoadInt8x16((*[16]int8)(unsafe.Pointer(&vecs[base+8])))

		f0 := lo.ExtendLo8ToInt32().ConvertToFloat32() // dims 0..7
		f1 := hi.ExtendLo8ToInt32().ConvertToFloat32() // dims 8..15

		d0 := f0.Mul(invS).Sub(q0)
		d1 := f1.Mul(invS).Sub(q1)

		acc := d0.Mul(d0)
		acc = d1.MulAdd(d1, acc) // acc = d1*d1 + acc
		out[j] = hsum8(acc)
	}
}

// hsum8 reduces a Float32x8 to the sum of its lanes.
func hsum8(v archsimd.Float32x8) float32 {
	s := v.GetLo().Add(v.GetHi())
	var a [4]float32
	s.Store(&a)
	return a[0] + a[1] + a[2] + a[3]
}
