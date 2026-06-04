package index

import (
	"fmt"
	"io"
	"math/rand"
	"runtime"
	"sync"
	"unsafe"
)

// BuildParams controls IVF construction.
type BuildParams struct {
	NClusters   int   // number of inverted lists (k-means k)
	KMeansIters int   // Lloyd iterations on the training sample
	SampleSize  int   // training sample size (<= N); 0 means use all
	NProbe      int   // default clusters scanned per query (tuned later)
	Seed        int64 // RNG seed for reproducible builds
}

// Build trains an IVF index over vecs (a flat N*PaddedDim float32 array in the
// original feature space, zero-padded) with per-reference fraud labels, then
// quantizes and reorders the vectors into cluster-grouped layout. The returned
// Index is in-memory and ready to Search or WriteTo.
func Build(vecs []float32, labels []bool, p BuildParams) (*Index, error) {
	n := len(labels)
	if n == 0 {
		return nil, fmt.Errorf("index: no reference vectors")
	}
	if len(vecs) != n*PaddedDim {
		return nil, fmt.Errorf("index: vecs len %d != %d*%d", len(vecs), n, PaddedDim)
	}
	if p.NClusters < 1 {
		p.NClusters = 1
	}
	if p.NClusters > n {
		p.NClusters = n
	}
	if p.NProbe < 1 || p.NProbe > p.NClusters {
		p.NProbe = min(p.NProbe, p.NClusters)
		if p.NProbe < 1 {
			p.NProbe = min(16, p.NClusters)
		}
	}
	rng := rand.New(rand.NewSource(p.Seed))

	centroids := kmeans(vecs, n, p, rng)

	// Assign every reference to its nearest centroid.
	assign := make([]uint32, n)
	assignNearest(vecs, n, nil, centroids, p.NClusters, assign)

	// Counting sort into cluster-grouped layout.
	counts := make([]int, p.NClusters)
	for _, c := range assign {
		counts[c]++
	}
	offsets := make([]uint32, p.NClusters+1)
	for c := 0; c < p.NClusters; c++ {
		offsets[c+1] = offsets[c] + uint32(counts[c])
	}
	cursor := make([]uint32, p.NClusters)
	copy(cursor, offsets[:p.NClusters])

	vectors := make([]int8, (n+guardVecs)*PaddedDim) // guard rows stay zero
	labelBits := make([]byte, (n+7)/8)
	for i := 0; i < n; i++ {
		c := assign[i]
		dst := int(cursor[c])
		cursor[c]++
		q := QuantizeVec(vecs[i*PaddedDim : i*PaddedDim+PaddedDim])
		copy(vectors[dst*PaddedDim:dst*PaddedDim+PaddedDim], q[:])
		if labels[i] {
			labelBits[dst>>3] |= 1 << uint(dst&7)
		}
	}

	hdr := Header{
		Count:     uint64(n),
		NClusters: uint32(p.NClusters),
		NProbe:    uint32(p.NProbe),
		Scale:     QuantScale,
	}
	return newInMemory(hdr, centroids, offsets, vectors, labelBits), nil
}

// kmeans trains NClusters centroids on a random sample of the data using
// Lloyd's algorithm, returning a flat NClusters*PaddedDim float32 array.
func kmeans(vecs []float32, n int, p BuildParams, rng *rand.Rand) []float32 {
	sample := p.SampleSize
	if sample <= 0 || sample > n {
		sample = n
	}
	sampleIdx := make([]int, sample)
	if sample == n {
		for i := range sampleIdx {
			sampleIdx[i] = i
		}
	} else {
		perm := rng.Perm(n)
		copy(sampleIdx, perm[:sample])
	}

	// Initialise centroids from distinct random sample points.
	centroids := make([]float32, p.NClusters*PaddedDim)
	init := rng.Perm(sample)
	for c := 0; c < p.NClusters; c++ {
		src := sampleIdx[init[c]] * PaddedDim
		copy(centroids[c*PaddedDim:c*PaddedDim+PaddedDim], vecs[src:src+PaddedDim])
	}

	assign := make([]uint32, sample) // assignment over sample positions
	sums := make([]float64, p.NClusters*PaddedDim)
	cnts := make([]int, p.NClusters)
	for iter := 0; iter < p.KMeansIters; iter++ {
		assignNearest(vecs, sample, sampleIdx, centroids, p.NClusters, assign)

		for i := range sums {
			sums[i] = 0
		}
		for i := range cnts {
			cnts[i] = 0
		}
		for s := 0; s < sample; s++ {
			c := int(assign[s])
			cnts[c]++
			base := sampleIdx[s] * PaddedDim
			cb := c * PaddedDim
			for d := 0; d < PaddedDim; d++ {
				sums[cb+d] += float64(vecs[base+d])
			}
		}
		for c := 0; c < p.NClusters; c++ {
			if cnts[c] == 0 {
				// Re-seed an empty cluster from a random sample point.
				src := sampleIdx[rng.Intn(sample)] * PaddedDim
				copy(centroids[c*PaddedDim:c*PaddedDim+PaddedDim], vecs[src:src+PaddedDim])
				continue
			}
			inv := 1.0 / float64(cnts[c])
			cb := c * PaddedDim
			for d := 0; d < PaddedDim; d++ {
				centroids[cb+d] = float32(sums[cb+d] * inv)
			}
		}
	}
	return centroids
}

// assignNearest assigns each selected point to its nearest centroid. If idxs is
// nil it processes points 0..count-1 and writes out[i]; otherwise it processes
// idxs[j], writing out[j]. The scan is parallelised across CPUs.
func assignNearest(vecs []float32, count int, idxs []int, centroids []float32, nClusters int, out []uint32) {
	total := count
	if idxs != nil {
		total = len(idxs)
	}
	parallelFor(total, func(lo, hi int) {
		dist := make([]float32, nClusters) // per-worker scratch
		var q [PaddedDim]float32
		for j := lo; j < hi; j++ {
			p := j
			if idxs != nil {
				p = idxs[j]
			}
			copy(q[:], vecs[p*PaddedDim:p*PaddedDim+PaddedDim])
			centDistBatch(&q, centroids, nClusters, dist)
			best, bestD := 0, dist[0]
			for c := 1; c < nClusters; c++ {
				if dist[c] < bestD {
					best, bestD = c, dist[c]
				}
			}
			out[j] = uint32(best)
		}
	})
}

// parallelFor splits [0,n) into contiguous ranges, one per worker goroutine.
func parallelFor(n int, fn func(lo, hi int)) {
	if n == 0 {
		return
	}
	workers := runtime.GOMAXPROCS(0)
	if workers > n {
		workers = n
	}
	if workers <= 1 {
		fn(0, n)
		return
	}
	var wg sync.WaitGroup
	chunk := (n + workers - 1) / workers
	for w := 0; w < workers; w++ {
		lo := w * chunk
		if lo >= n {
			break
		}
		hi := lo + chunk
		if hi > n {
			hi = n
		}
		wg.Add(1)
		go func(lo, hi int) {
			defer wg.Done()
			fn(lo, hi)
		}(lo, hi)
	}
	wg.Wait()
}

// WriteTo serializes the index to w in the references.bin format.
func (ix *Index) WriteTo(w io.Writer) (int64, error) {
	l := computeLayout(ix.hdr.NClusters, ix.hdr.Count)
	var written int64

	hdr := ix.hdr.encode()
	if err := writeAll(w, hdr[:], &written); err != nil {
		return written, err
	}
	if err := writeAll(w, bytesOf(ix.centroids), &written); err != nil {
		return written, err
	}
	if err := writeAll(w, bytesOf(ix.offsets), &written); err != nil {
		return written, err
	}
	if pad := l.vectorsOff - written; pad > 0 {
		if err := writeAll(w, make([]byte, pad), &written); err != nil {
			return written, err
		}
	}
	if err := writeAll(w, bytesOfInt8(ix.vectors), &written); err != nil {
		return written, err
	}
	if err := writeAll(w, ix.labels, &written); err != nil {
		return written, err
	}
	if written != l.total {
		return written, fmt.Errorf("index: wrote %d bytes, expected %d", written, l.total)
	}
	return written, nil
}

func writeAll(w io.Writer, b []byte, written *int64) error {
	n, err := w.Write(b)
	*written += int64(n)
	return err
}

func bytesOf[T uint32 | float32](s []T) []byte {
	if len(s) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&s[0])), len(s)*4)
}

func bytesOfInt8(s []int8) []byte {
	if len(s) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&s[0])), len(s))
}
