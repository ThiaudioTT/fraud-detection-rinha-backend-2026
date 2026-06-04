package index

import (
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

// makeData generates n synthetic 14-dim references (padded to PaddedDim), with a
// fraction of -1 sentinels at dims 5/6 and random fraud labels.
func makeData(n int, seed int64) (vecs []float32, labels []bool, queries [][]float32) {
	rng := rand.New(rand.NewSource(seed))
	vecs = make([]float32, n*PaddedDim)
	labels = make([]bool, n)
	for i := 0; i < n; i++ {
		base := i * PaddedDim
		for d := 0; d < Dim; d++ {
			vecs[base+d] = rng.Float32()
		}
		if rng.Float32() < 0.3 {
			vecs[base+5], vecs[base+6] = -1, -1
		}
		labels[i] = rng.Float32() < 0.4
	}
	queries = make([][]float32, 200)
	for i := range queries {
		q := make([]float32, Dim)
		for d := 0; d < Dim; d++ {
			q[d] = rng.Float32()
		}
		if rng.Float32() < 0.3 {
			q[5], q[6] = -1, -1
		}
		queries[i] = q
	}
	return
}

// refTopK is the independent oracle: exact k-NN using the same asymmetric metric
// (full-precision query vs dequantized int8 reference).
func refTopK(query []float32, vecs []float32, labels []bool, n, k int) NeighborStats {
	type cand struct {
		d float32
		f bool
	}
	cands := make([]cand, n)
	q := padQuery(query)
	for i := 0; i < n; i++ {
		var sum float32
		base := i * PaddedDim
		for d := 0; d < PaddedDim; d++ {
			 qd := float32(Quantize(vecs[base+d]))*invScale - q[d]
			sum += qd * qd
		}
		cands[i] = cand{sum, labels[i]}
	}
	// partial selection of k smallest
	for a := 0; a < k && a < n; a++ {
		mi := a
		for b := a + 1; b < n; b++ {
			if cands[b].d < cands[mi].d {
				mi = b
			}
		}
		cands[a], cands[mi] = cands[mi], cands[a]
	}
	st := NeighborStats{Total: min(k, n)}
	for a := 0; a < st.Total; a++ {
		if cands[a].f {
			st.Fraud++
		}
	}
	return st
}

func TestSearchBruteMatchesOracle(t *testing.T) {
	const n, k = 3000, 5
	vecs, labels, queries := makeData(n, 1)
	ix, err := Build(vecs, labels, BuildParams{NClusters: 32, KMeansIters: 8, NProbe: 32, Seed: 7})
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range queries {
		got := ix.SearchBrute(q, k)
		want := refTopK(q, vecs, labels, n, k)
		if got != want {
			t.Fatalf("brute mismatch: got %+v want %+v", got, want)
		}
	}
}

func TestIVFFullProbeEqualsBrute(t *testing.T) {
	// With NProbe == NClusters the IVF scans every cluster, so it must match the
	// exact brute-force scan decision-for-decision.
	const n, k = 3000, 5
	vecs, labels, queries := makeData(n, 2)
	nClusters := 32
	ix, err := Build(vecs, labels, BuildParams{NClusters: nClusters, KMeansIters: 8, NProbe: nClusters, Seed: 9})
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range queries {
		if ivf, brute := ix.Search(q, k), ix.SearchBrute(q, k); ivf != brute {
			t.Fatalf("ivf(full) %+v != brute %+v", ivf, brute)
		}
	}
}

func TestRoundTripMmap(t *testing.T) {
	const n, k = 5000, 5
	vecs, labels, queries := makeData(n, 3)
	ix, err := Build(vecs, labels, BuildParams{NClusters: 64, KMeansIters: 10, NProbe: 16, Seed: 11})
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "references.bin")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ix.WriteTo(f); err != nil {
		t.Fatal(err)
	}
	f.Close()

	mix, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer mix.Close()

	if mix.Count() != n {
		t.Fatalf("count: got %d want %d", mix.Count(), n)
	}
	for _, q := range queries {
		if a, b := mix.Search(q, k), ix.Search(q, k); a != b {
			t.Fatalf("mmap vs in-memory search mismatch: %+v vs %+v", a, b)
		}
	}
}

func TestIVFRecallAgainstBrute(t *testing.T) {
	// A modest nprobe should agree with the exact decision on the large majority
	// of queries; this guards against a broken IVF (e.g. centroid/offset bug).
	const n, k = 20000, 5
	vecs, labels, queries := makeData(n, 4)
	ix, err := Build(vecs, labels, BuildParams{NClusters: 128, KMeansIters: 12, SampleSize: 8000, NProbe: 24, Seed: 5})
	if err != nil {
		t.Fatal(err)
	}
	agree := 0
	for _, q := range queries {
		if ix.Search(q, k).approved() == ix.SearchBrute(q, k).approved() {
			agree++
		}
	}
	rate := float64(agree) / float64(len(queries))
	if rate < 0.9 {
		t.Fatalf("IVF/brute decision agreement too low: %.3f", rate)
	}
	t.Logf("decision agreement nprobe=24/128: %.3f", rate)
}

// approved mirrors the service decision rule for test assertions.
func (s NeighborStats) approved() bool {
	if s.Total == 0 {
		return true
	}
	return float64(s.Fraud)/float64(s.Total) < 0.6
}

func TestQuantizeRange(t *testing.T) {
	cases := map[float32]int8{-1: -127, 0: 0, 1: 127, 0.5: 64, -0.5: -64}
	for in, want := range cases {
		if got := Quantize(in); got != want {
			t.Errorf("Quantize(%v)=%d want %d", in, got, want)
		}
	}
	if got := Quantize(2); got != 127 {
		t.Errorf("clamp hi: %d", got)
	}
	if got := Quantize(-3); got != -127 {
		t.Errorf("clamp lo: %d", got)
	}
	_ = math.Sqrt
}
