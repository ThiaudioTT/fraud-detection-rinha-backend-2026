// Command preprocess turns the immutable references.json.gz dataset into the
// compact references.bin (int8 IVF index) that the API mmaps at runtime. It runs
// once at image-build time, not on the request path.
//
// It streams the 3M {vector,label} records, quantizes and clusters them, tunes
// nprobe against an exact brute-force oracle to hit a recall target, and writes
// the binary. Quantization/clustering/recall figures are logged so a build can
// be judged before it ships.
package main

import (
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"

	"fraud-detection-2026/internal/index"
)

const defaultURL = "https://raw.githubusercontent.com/zanfranceschi/rinha-de-backend-2026/main/resources/references.json.gz"

type reference struct {
	Vector []float64 `json:"vector"`
	Label  string    `json:"label"`
}

func main() {
	var (
		input    = flag.String("input", defaultURL, "references.json.gz URL or local path")
		output   = flag.String("output", "references.bin", "output binary path")
		clusters = flag.Int("clusters", 2048, "IVF clusters (k-means k)")
		iters    = flag.Int("iters", 15, "k-means iterations")
		sample   = flag.Int("sample", 200_000, "k-means training sample size")
		nprobe0  = flag.Int("nprobe", 16, "starting nprobe for tuning")
		target   = flag.Float64("target", 0.995, "min IVF/brute decision agreement")
		valN     = flag.Int("validate", 3000, "validation queries sampled from the dataset")
		seed     = flag.Int64("seed", 42, "RNG seed (reproducible builds)")
	)
	flag.Parse()

	start := time.Now()
	vecs, labels, err := load(*input)
	if err != nil {
		log.Fatalf("load: %v", err)
	}
	n := len(labels)
	log.Printf("loaded %d references in %v", n, time.Since(start))

	buildStart := time.Now()
	ix, err := index.Build(vecs, labels, index.BuildParams{
		NClusters:   *clusters,
		KMeansIters: *iters,
		SampleSize:  *sample,
		NProbe:      *nprobe0,
		Seed:        *seed,
	})
	if err != nil {
		log.Fatalf("build: %v", err)
	}
	log.Printf("built IVF (%d clusters) in %v", *clusters, time.Since(buildStart))

	tuneNProbe(ix, vecs, labels, *valN, *target, *seed)

	if err := write(ix, *output); err != nil {
		log.Fatalf("write: %v", err)
	}
	log.Printf("wrote %s (nprobe=%d) — total %v", *output, ix.NProbe(), time.Since(start))
}

// load streams the gzipped JSON array of {vector,label} into a flat padded
// float32 array and a label slice.
func load(src string) ([]float32, []bool, error) {
	var r io.ReadCloser
	if len(src) > 4 && src[:4] == "http" {
		resp, err := http.Get(src)
		if err != nil {
			return nil, nil, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, nil, fmt.Errorf("GET %s: %s", src, resp.Status)
		}
		r = resp.Body
	} else {
		f, err := os.Open(src)
		if err != nil {
			return nil, nil, err
		}
		r = f
	}
	defer r.Close()

	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, nil, err
	}
	defer gz.Close()

	dec := json.NewDecoder(gz)
	if _, err := dec.Token(); err != nil { // opening '['
		return nil, nil, err
	}

	vecs := make([]float32, 0, 3_000_000*index.PaddedDim)
	labels := make([]bool, 0, 3_000_000)
	var padded [index.PaddedDim]float32
	for dec.More() {
		var ref reference
		if err := dec.Decode(&ref); err != nil {
			return nil, nil, err
		}
		if len(ref.Vector) != index.Dim {
			return nil, nil, fmt.Errorf("record %d has %d dims, want %d", len(labels), len(ref.Vector), index.Dim)
		}
		padded = [index.PaddedDim]float32{}
		for d := 0; d < index.Dim; d++ {
			padded[d] = float32(ref.Vector[d])
		}
		vecs = append(vecs, padded[:]...)
		labels = append(labels, ref.Label == "fraud")
		if len(labels)%500_000 == 0 {
			log.Printf("  streamed %d...", len(labels))
		}
	}
	return vecs, labels, nil
}

// tuneNProbe raises nprobe until IVF decisions agree with the exact int8
// brute-force oracle on at least `target` of a validation sample, and reports
// agreement against the full-precision (float) exact answer as the absolute
// accuracy proxy. The chosen nprobe is stored in the index header. All scans are
// parallelised across CPUs since the oracles are O(N) per query.
func tuneNProbe(ix *index.Index, vecs []float32, labels []bool, valN int, target float64, seed int64) {
	n := len(labels)
	if valN > n {
		valN = n
	}
	rng := rand.New(rand.NewSource(seed + 1))
	qIdx := rng.Perm(n)[:valN]
	queryOf := func(i int) []float32 {
		gi := qIdx[i]
		return vecs[gi*index.PaddedDim : gi*index.PaddedDim+index.Dim]
	}

	// Precompute the oracle decisions once (the expensive part).
	bruteApproved := make([]bool, valN)
	floatApproved := make([]bool, valN)
	parallelMap(valN, func(i int) {
		bruteApproved[i] = approved(ix.SearchBrute(queryOf(i), 5))
		floatApproved[i] = approved(floatExactTop5(queryOf(i), vecs, labels, n))
	})

	// Evaluate the full ladder so the recall curve is visible in build logs, and
	// select the cheapest nprobe that meets the target against the (more relevant)
	// full-precision oracle — fewer clusters scanned means less per-query CPU.
	candidates := []int{2, 4, 6, 8, 12, 16, 24, 32, 48, 64, 96, 128}
	chosen, picked := candidates[len(candidates)-1], false
	for _, np := range candidates {
		ix.SetNProbe(np)
		var agreeBrute, agreeFloat int64
		ivfApproved := make([]bool, valN)
		parallelMap(valN, func(i int) {
			ivfApproved[i] = approved(ix.Search(queryOf(i), 5))
		})
		for i := 0; i < valN; i++ {
			if ivfApproved[i] == bruteApproved[i] {
				agreeBrute++
			}
			if ivfApproved[i] == floatApproved[i] {
				agreeFloat++
			}
		}
		rb := float64(agreeBrute) / float64(valN)
		rf := float64(agreeFloat) / float64(valN)
		log.Printf("  nprobe=%-3d  vs-brute=%.4f  vs-float-exact=%.4f", np, rb, rf)
		if !picked && rf >= target {
			chosen, picked = np, true
		}
	}
	ix.SetNProbe(chosen)
	log.Printf("selected nprobe=%d (target vs-float-exact %.3f)", chosen, target)
}

// parallelMap runs fn(0..n-1) across GOMAXPROCS goroutines.
func parallelMap(n int, fn func(i int)) {
	workers := runtime.GOMAXPROCS(0)
	if workers > n {
		workers = n
	}
	if workers < 1 {
		workers = 1
	}
	var wg sync.WaitGroup
	next := make(chan int, workers)
	go func() {
		for i := 0; i < n; i++ {
			next <- i
		}
		close(next)
	}()
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				fn(i)
			}
		}()
	}
	wg.Wait()
}

// floatExactTop5 is the full-precision oracle: exact L2 over the un-quantized
// float references (this is the closest stand-in for the dataset's "true"
// nearest neighbours).
func floatExactTop5(query []float32, vecs []float32, labels []bool, n int) index.NeighborStats {
	const k = 5
	var topD [k]float32
	var topF [k]bool
	cnt := 0
	worst, worstV := 0, float32(0)
	for i := 0; i < n; i++ {
		base := i * index.PaddedDim
		var sum float32
		for d := 0; d < index.Dim; d++ {
			df := vecs[base+d] - query[d]
			sum += df * df
		}
		if cnt < k {
			topD[cnt], topF[cnt] = sum, labels[i]
			cnt++
			if cnt == k {
				worst, worstV = argmax(topD[:])
			}
			continue
		}
		if sum < worstV {
			topD[worst], topF[worst] = sum, labels[i]
			worst, worstV = argmax(topD[:])
		}
	}
	st := index.NeighborStats{Total: cnt}
	for i := 0; i < cnt; i++ {
		if topF[i] {
			st.Fraud++
		}
	}
	return st
}

func argmax(s []float32) (int, float32) {
	mi, mv := 0, s[0]
	for i := 1; i < len(s); i++ {
		if s[i] > mv {
			mi, mv = i, s[i]
		}
	}
	return mi, mv
}

func approved(s index.NeighborStats) bool {
	if s.Total == 0 {
		return true
	}
	return float64(s.Fraud)/float64(s.Total) < 0.6
}

func write(ix *index.Index, path string) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := ix.WriteTo(f); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
