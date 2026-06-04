package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fraud-detection-2026/internal/handlers"
	"fraud-detection-2026/internal/repo"
	"fraud-detection-2026/internal/service"
	"fraud-detection-2026/pkg/database"

	"github.com/gin-gonic/gin"
)

func main() {
	ctx := context.Background()

	pool, err := database.Connect(ctx)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	// Compose the dependency graph once, at startup.
	referenceRepo := repo.NewReferenceRepo(pool)
	fraudService := service.NewFraudService(referenceRepo)
	fraudHandler := handlers.NewFraudHandler(fraudService)
	readyHandler := handlers.NewReadyHandler(pool)

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
	// requests drain before the pool closes.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}
