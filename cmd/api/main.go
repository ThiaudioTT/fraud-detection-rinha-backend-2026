package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"fraud-detection-2026/internal/handlers"
	"fraud-detection-2026/internal/index"
	"fraud-detection-2026/internal/repo"
	"fraud-detection-2026/internal/service"

	"github.com/gin-gonic/gin"
)

func main() {
	// Load the reference index (mmap'd, read-only). The pages live off the Go
	// heap and are shared across instances mapping the same image layer.
	path := os.Getenv("REFERENCES_PATH")
	if path == "" {
		path = "/app/references.bin"
	}
	idx, err := index.Open(path)
	if err != nil {
		log.Fatalf("index: %v", err)
	}
	defer idx.Close()

	// Optional runtime override of the baked-in nprobe, to trade recall for
	// per-query CPU without rebuilding the index.
	if v := os.Getenv("NPROBE"); v != "" {
		if n, perr := strconv.Atoi(v); perr == nil && n > 0 {
			idx.SetNProbe(n)
		}
	}

	// Self-test before announcing readiness: a zero query must return neighbours.
	if st := idx.Search(make([]float32, index.Dim), 5); st.Total == 0 {
		log.Fatalf("index self-test returned no neighbours (count=%d)", idx.Count())
	}
	log.Printf("index ready: %d vectors, nprobe=%d", idx.Count(), idx.NProbe())

	// Compose the dependency graph once, at startup.
	referenceRepo := repo.NewReferenceRepo(idx)
	fraudService := service.NewFraudService(referenceRepo)
	fraudHandler := handlers.NewFraudHandler(fraudService)
	readyHandler := handlers.NewReadyHandler(idx)

	// Release mode + no default Logger middleware: per-request stdout logging is
	// the single biggest throughput sink under load. Recovery stays so a panic
	// in one request can't take the instance down.
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/ready", readyHandler.Ready)
	r.POST("/fraud-score", fraudHandler.FraudScore)

	srv := &http.Server{
		Addr:         ":9999",
		Handler:      r,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()
	log.Println("listening on :9999")

	// Graceful shutdown: stop accepting new connections and let in-flight
	// requests drain before exiting.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}
