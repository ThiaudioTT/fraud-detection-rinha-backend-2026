// Package index implements the in-process vector search that replaces the
// previous Postgres/pgvector backend. The 3M reference vectors are quantized to
// int8, grouped into an IVF (inverted-file) index, and serialized to a compact
// binary blob (references.bin) that is baked into the image at build time and
// mmap'd read-only at runtime. Keeping the data off the Go heap (raw mmap +
// unsafe slices) means the garbage collector never scans 3M vectors, and the
// page cache for the file is shared across both API containers (same inode in
// the shared image layer).
//
// The on-disk format is little-endian and is only ever produced and consumed on
// linux/amd64, so the reader reinterprets mmap'd bytes with native-endian
// unsafe slices. Do not read these files on a big-endian host.
package index

import (
	"encoding/binary"
	"fmt"
)

const (
	// Dim is the number of real feature dimensions (see DETECTION_RULES.md).
	Dim = 14
	// PaddedDim rounds Dim up to a multiple of 16 so each vector occupies a
	// clean number of SIMD lanes; the trailing dimensions are always zero and
	// therefore contribute nothing to the L2 distance.
	PaddedDim = 16
	// QuantScale maps the reference value range [-1, 1] onto int8 [-127, 127].
	// -1 (the "no prior transaction" sentinel) maps to -127, 0 to 0, 1 to 127.
	QuantScale = 127.0

	invScale = float32(1.0 / QuantScale)

	magicValue = 0x52564231 // "RVB1"
	formatVer  = 1
	headerSize = 64
	alignBytes = 64

	// guardVecs trailing zero vectors pad the vectors region so the AVX2 kernel,
	// which loads the two 8-lane halves of a vector from separate pointers, can
	// over-read up to 8 bytes past the last real vector without leaving the
	// mapping. The guard rows are never searched (ranges are bounded by Count).
	guardVecs = 1
)

// Header is the fixed-size prefix of references.bin. It is serialized field by
// field (not by struct layout) so the encoding is independent of Go's struct
// padding.
type Header struct {
	Count     uint64
	NClusters uint32
	NProbe    uint32 // default nprobe chosen at build time to hit the recall target
	Scale     float32
}

func (h Header) encode() [headerSize]byte {
	var b [headerSize]byte
	binary.LittleEndian.PutUint32(b[0:], magicValue)
	binary.LittleEndian.PutUint32(b[4:], formatVer)
	binary.LittleEndian.PutUint64(b[8:], h.Count)
	binary.LittleEndian.PutUint32(b[16:], Dim)
	binary.LittleEndian.PutUint32(b[20:], PaddedDim)
	binary.LittleEndian.PutUint32(b[24:], uint32(h.NClusters))
	binary.LittleEndian.PutUint32(b[28:], h.NProbe)
	binary.LittleEndian.PutUint32(b[32:], uint32(h.Scale))
	// bytes [36:64] reserved / zero
	return b
}

func decodeHeader(b []byte) (Header, error) {
	if len(b) < headerSize {
		return Header{}, fmt.Errorf("index: file shorter than header (%d bytes)", len(b))
	}
	if got := binary.LittleEndian.Uint32(b[0:]); got != magicValue {
		return Header{}, fmt.Errorf("index: bad magic 0x%08x", got)
	}
	if got := binary.LittleEndian.Uint32(b[4:]); got != formatVer {
		return Header{}, fmt.Errorf("index: unsupported format version %d", got)
	}
	if got := binary.LittleEndian.Uint32(b[16:]); got != Dim {
		return Header{}, fmt.Errorf("index: dim mismatch (file %d, build %d)", got, Dim)
	}
	if got := binary.LittleEndian.Uint32(b[20:]); got != PaddedDim {
		return Header{}, fmt.Errorf("index: padded-dim mismatch (file %d, build %d)", got, PaddedDim)
	}
	h := Header{
		Count:     binary.LittleEndian.Uint64(b[8:]),
		NClusters: binary.LittleEndian.Uint32(b[24:]),
		NProbe:    binary.LittleEndian.Uint32(b[28:]),
		Scale:     float32(binary.LittleEndian.Uint32(b[32:])),
	}
	return h, nil
}

// layout returns the byte offsets of each region for the given counts. The
// vectors region is padded up to alignBytes so int8 loads stay aligned.
type layout struct {
	centroidsOff int64
	offsetsOff   int64
	vectorsOff   int64
	labelsOff    int64
	total        int64
}

func computeLayout(nClusters uint32, count uint64) layout {
	var l layout
	l.centroidsOff = headerSize
	centroidsLen := int64(nClusters) * PaddedDim * 4 // float32
	l.offsetsOff = l.centroidsOff + centroidsLen
	offsetsLen := int64(nClusters+1) * 4 // uint32 prefix sums
	l.vectorsOff = alignUp(l.offsetsOff+offsetsLen, alignBytes)
	vectorsLen := int64(count+guardVecs) * PaddedDim // int8, incl. guard rows
	l.labelsOff = l.vectorsOff + vectorsLen
	labelsLen := int64((count + 7) / 8) // 1 bit per reference
	l.total = l.labelsOff + labelsLen
	return l
}

func alignUp(n, a int64) int64 {
	return (n + a - 1) / a * a
}
