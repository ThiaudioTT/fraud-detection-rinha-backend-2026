package main

import (
	"context"
	"log"

	"fraud-detection-2026/internal/handlers"
	"fraud-detection-2026/pkg/database"

	"github.com/gin-gonic/gin"
)

func main() {
	if err := database.Connect(context.Background()); err != nil {
		log.Fatalf("database: %v", err)
	}
	defer database.Close()

	r := gin.Default()

	r.GET("/ready", handlers.Ready)
	r.POST("/fraud-score", handlers.FraudScore)

	r.Run(":9999")
}
