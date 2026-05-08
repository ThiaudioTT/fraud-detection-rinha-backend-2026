package main

import (
	"context"
	"log"

	"fraud-detection-2026/pkg/database"
	"fraud-detection-2026/scripts"
)

func main() {
	ctx := context.Background()
	if err := database.Connect(ctx); err != nil {
		log.Fatalf("database: %v", err)
	}
	defer database.Close()

	n, err := scripts.LoadVectorRefs(ctx, database.Pool)
	if err != nil {
		log.Fatalf("load refs: %v", err)
	}
	if n == 0 {
		log.Print("reference_vectors already populated, skipping")
		return
	}
	log.Printf("loaded %d reference vectors", n)
}
