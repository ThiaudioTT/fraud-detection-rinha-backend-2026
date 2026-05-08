package scripts

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

const referencesURL = "https://raw.githubusercontent.com/zanfranceschi/rinha-de-backend-2026/main/resources/references.json.gz"

// LoadVectorRefs streams references.json.gz into reference_vectors via COPY.
// No-op if the table is already populated.
func LoadVectorRefs(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	var populated bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM reference_vectors)").Scan(&populated); err != nil {
		return 0, err
	}
	if populated {
		return 0, nil
	}

	resp, err := http.Get(referencesURL)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("references: %s", resp.Status)
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return 0, err
	}
	defer gz.Close()

	dec := json.NewDecoder(gz)
	if _, err := dec.Token(); err != nil { // consume '['
		return 0, err
	}

	return pool.CopyFrom(ctx,
		pgx.Identifier{"reference_vectors"},
		[]string{"vector", "is_fraud"},
		pgx.CopyFromFunc(func() ([]any, error) {
			if !dec.More() {
				return nil, nil
			}
			var rec struct {
				Vector []float32 `json:"vector"`
				Label  string    `json:"label"`
			}
			if err := dec.Decode(&rec); err != nil {
				return nil, err
			}
			return []any{pgvector.NewVector(rec.Vector), rec.Label == "fraud"}, nil
		}),
	)
}
