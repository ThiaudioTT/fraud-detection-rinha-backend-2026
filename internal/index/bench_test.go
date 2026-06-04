package index

import (
	"math/rand"
	"sync"
	"testing"
)

var (
	benchOnce sync.Once
	benchIx   *Index
	benchQs   [][]float32
)

func benchIndex(b *testing.B) (*Index, [][]float32) {
	benchOnce.Do(func() {
		const n = 1_000_000
		vecs, labels, _ := makeData(n, 99)
		ix, err := Build(vecs, labels, BuildParams{NClusters: 1024, KMeansIters: 12, SampleSize: 150_000, NProbe: 16, Seed: 1})
		if err != nil {
			b.Fatal(err)
		}
		benchIx = ix
		qs := make([][]float32, 1000)
		rng := rand.New(rand.NewSource(7))
		for i := range qs {
			q := make([]float32, Dim)
			for d := range q {
				q[d] = rng.Float32()
			}
			qs[i] = q
		}
		benchQs = qs
	})
	return benchIx, benchQs
}

func BenchmarkSearchIVF(b *testing.B) {
	ix, qs := benchIndex(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ix.Search(qs[i%len(qs)], 5)
	}
}

func BenchmarkSearchBrute(b *testing.B) {
	ix, qs := benchIndex(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ix.SearchBrute(qs[i%len(qs)], 5)
	}
}
