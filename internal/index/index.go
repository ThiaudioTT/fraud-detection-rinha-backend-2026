package index

import (
	"fmt"
	"sync"
	"unsafe"
)

// Index is a read-only IVF index over int8-quantized reference vectors. It can
// be backed either by an mmap'd references.bin (runtime) or by in-memory arrays
// (build-time validation); the search code is identical for both.
type Index struct {
	hdr       Header
	centroids []float32 // NClusters * PaddedDim, original (float) space
	offsets   []uint32  // NClusters+1 prefix sums into vectors
	vectors   []int8    // Count * PaddedDim, cluster-grouped, quantized
	labels    []byte    // Count bits, cluster-grouped (1 = fraud)

	mmapData []byte // underlying mapping to unmap on Close; nil if in-memory
	scratch  sync.Pool
}

// NeighborStats summarises the k nearest references: how many are fraud and how
// many were returned in total.
type NeighborStats struct {
	Fraud int
	Total int
}

// chunkSize bounds the per-query distance scratch so a huge cluster does not
// force a huge allocation; cluster lists are scanned chunk by chunk.
const chunkSize = 2048

type searchScratch struct {
	centDist []float32   // NClusters
	probes   nearestSet  // nprobe smallest centroid distances
	out      []float32   // chunkSize distances
	nn       nearestSet  // k nearest references
}

func (ix *Index) newScratch() *searchScratch {
	s := &searchScratch{
		centDist: make([]float32, ix.hdr.NClusters),
		out:      make([]float32, chunkSize),
	}
	return s
}

func newInMemory(hdr Header, centroids []float32, offsets []uint32, vectors []int8, labels []byte) *Index {
	ix := &Index{hdr: hdr, centroids: centroids, offsets: offsets, vectors: vectors, labels: labels}
	ix.scratch.New = func() any { return ix.newScratch() }
	return ix
}

// Count reports the number of reference vectors in the index.
func (ix *Index) Count() int { return int(ix.hdr.Count) }

// NProbe reports the configured number of clusters scanned per query.
func (ix *Index) NProbe() int { return int(ix.hdr.NProbe) }

// SetNProbe overrides the number of clusters scanned per query (used while
// tuning at build time).
func (ix *Index) SetNProbe(n int) { ix.hdr.NProbe = uint32(n) }

// Search returns fraud/total over the k nearest references using the IVF index:
// it scans only the NProbe clusters whose centroids are closest to the query.
func (ix *Index) Search(query []float32, k int) NeighborStats {
	q := padQuery(query)
	s := ix.scratch.Get().(*searchScratch)
	defer ix.scratch.Put(s)

	// Pick the NProbe nearest centroids.
	nprobe := int(ix.hdr.NProbe)
	s.probes.reset(nprobe)
	nc := int(ix.hdr.NClusters)
	centDistBatch(&q, ix.centroids, nc, s.centDist)
	for c := 0; c < nc; c++ {
		s.probes.consider(int32(c), s.centDist[c])
	}

	// Scan the candidate lists of the chosen clusters.
	s.nn.reset(k)
	for i := 0; i < s.probes.n; i++ {
		c := int(s.probes.idx[i])
		ix.scanRange(&q, int(ix.offsets[c]), int(ix.offsets[c+1]), s)
	}
	return ix.tally(s)
}

// SearchBrute scans every reference vector (exact over the int8 quantization).
// It is the runtime fallback and the build-time recall oracle for the IVF path.
func (ix *Index) SearchBrute(query []float32, k int) NeighborStats {
	q := padQuery(query)
	s := ix.scratch.Get().(*searchScratch)
	defer ix.scratch.Put(s)

	s.nn.reset(k)
	ix.scanRange(&q, 0, int(ix.hdr.Count), s)
	return ix.tally(s)
}

// scanRange computes distances for references [start,end) in chunks and folds
// them into the k-nearest set s.nn.
func (ix *Index) scanRange(q *[PaddedDim]float32, start, end int, s *searchScratch) {
	for base := start; base < end; base += chunkSize {
		n := end - base
		if n > chunkSize {
			n = chunkSize
		}
		distBatch(q, ix.vectors[base*PaddedDim:], n, s.out)
		for j := 0; j < n; j++ {
			s.nn.consider(int32(base+j), s.out[j])
		}
	}
}

func (ix *Index) tally(s *searchScratch) NeighborStats {
	stats := NeighborStats{Total: s.nn.n}
	for i := 0; i < s.nn.n; i++ {
		gi := int(s.nn.idx[i])
		if ix.labels[gi>>3]&(1<<uint(gi&7)) != 0 {
			stats.Fraud++
		}
	}
	return stats
}

// nearestSet keeps the k smallest (index, dist) pairs seen via consider. It is
// an unordered bag with a tracked worst slot: most candidates are rejected with
// a single comparison once the set is full.
type nearestSet struct {
	dist     []float32
	idx      []int32
	k        int
	n        int
	worst    int     // index of current max in dist[:n]; valid only when n == k
	worstVal float32
}

func (s *nearestSet) reset(k int) {
	if cap(s.dist) < k {
		s.dist = make([]float32, k)
		s.idx = make([]int32, k)
	}
	s.dist = s.dist[:k]
	s.idx = s.idx[:k]
	s.k = k
	s.n = 0
	s.worst = 0
	s.worstVal = 0
}

func (s *nearestSet) consider(i int32, d float32) {
	if s.n < s.k {
		s.dist[s.n] = d
		s.idx[s.n] = i
		s.n++
		if s.n == s.k {
			s.recomputeWorst()
		}
		return
	}
	if d >= s.worstVal {
		return
	}
	s.dist[s.worst] = d
	s.idx[s.worst] = i
	s.recomputeWorst()
}

func (s *nearestSet) recomputeWorst() {
	w, wv := 0, s.dist[0]
	for i := 1; i < s.n; i++ {
		if s.dist[i] > wv {
			w, wv = i, s.dist[i]
		}
	}
	s.worst, s.worstVal = w, wv
}

// fromMmap reinterprets an mmap'd references.bin (native little-endian) as typed
// slices without copying. The returned Index keeps data alive via mmapData.
func fromMmap(data []byte) (*Index, error) {
	hdr, err := decodeHeader(data)
	if err != nil {
		return nil, err
	}
	l := computeLayout(hdr.NClusters, hdr.Count)
	if int64(len(data)) < l.total {
		return nil, fmt.Errorf("index: file truncated (have %d, need %d)", len(data), l.total)
	}

	nCent := int(hdr.NClusters) * PaddedDim
	nVec := (int(hdr.Count) + guardVecs) * PaddedDim
	centroids := unsafe.Slice((*float32)(unsafe.Pointer(&data[l.centroidsOff])), nCent)
	offsets := unsafe.Slice((*uint32)(unsafe.Pointer(&data[l.offsetsOff])), int(hdr.NClusters)+1)
	vectors := unsafe.Slice((*int8)(unsafe.Pointer(&data[l.vectorsOff])), nVec)
	labels := data[l.labelsOff:l.total]

	ix := newInMemory(hdr, centroids, offsets, vectors, labels)
	ix.mmapData = data
	return ix, nil
}
