package main

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"fraud-detection-2026/pkg/database"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
)

const referencesURL = "https://raw.githubusercontent.com/zanfranceschi/rinha-de-backend-2026/main/resources/references.json.gz"

// Schema of the data inside this json ^
// [
//   { "vector": [0.01, 0.0833, ...], "label": "legit" },
//   { "vector": [0.5796, 0.9167, ...], "label": "fraud" }
// ]

type Reference struct {
	Vector []float64 `json:"vector"`
	Label  string    `json:"label"`
}

// referenceSource adapts a streaming JSON decoder to pgx.CopyFromSource so we
// can COPY the whole dataset in a single call without buffering it in memory.
type referenceSource struct {
	decoder *json.Decoder
	current Reference
	count   int
	err     error
}

func (s *referenceSource) Next() bool {
	if s.err != nil || !s.decoder.More() {
		return false
	}
	s.current = Reference{}
	if err := s.decoder.Decode(&s.current); err != nil {
		s.err = err
		return false
	}
	s.count++
	if s.count%100_000 == 0 {
		log.Printf("streamed %d rows...", s.count)
	}
	return true
}

func (s *referenceSource) Values() ([]any, error) {
	vec32 := make([]float32, len(s.current.Vector))
	for i, v := range s.current.Vector {
		vec32[i] = float32(v)
	}
	return []any{pgvector.NewVector(vec32), s.current.Label == "fraud"}, nil
}

func (s *referenceSource) Err() error { return s.err }

func SeedDb() {
	startAt := time.Now()

	ctx := context.Background()
	pool, err := database.Connect(ctx)
	if err != nil {
		panic(fmt.Sprintf("Failed to connect to database: %v", err))
	}
	defer pool.Close()

	conn, err := pool.Acquire(ctx)
	if err != nil {
		panic(err)
	}
	defer conn.Release()

	// Tune the session for bulk loading + parallel HNSW build.
	for _, stmt := range []string{
		"SET synchronous_commit = off",
		"SET maintenance_work_mem = '64MB'",
		"SET max_parallel_maintenance_workers = 0",
		"SET max_parallel_workers = 0",
	} {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			panic(err)
		}
	}

	log.Println("Downloading references...")
	resp, err := http.Get(referencesURL)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	gzReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		panic(err)
	}
	defer gzReader.Close()

	decoder := json.NewDecoder(gzReader)
	if _, err := decoder.Token(); err != nil { // opening '['
		panic(err)
	}

	src := &referenceSource{decoder: decoder}

	log.Println("Starting COPY...")
	copyStart := time.Now()
	n, err := conn.Conn().CopyFrom(
		ctx,
		pgx.Identifier{"reference_vectors"},
		[]string{"vector", "is_fraud"},
		src,
	)
	if err != nil {
		panic(fmt.Sprintf("copy failed after %d rows: %v", src.count, err))
	}
	log.Printf("COPY done: %d rows in %v", n, time.Since(copyStart))

	log.Println("Rebuilding HNSW index...")
	idxStart := time.Now()
	if _, err := conn.Exec(ctx, "CREATE INDEX reference_vectors_l2_idx ON reference_vectors USING hnsw (vector vector_l2_ops) WITH (m = 4, ef_construction = 16)"); err != nil {
		panic(err)
	}
	log.Printf("Index built in %v", time.Since(idxStart))

	fmt.Println("processed:", n, "duration:", time.Since(startAt))
}

func main() {
	log.Println("start seeding database...")
	SeedDb()
}
