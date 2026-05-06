package main

import (
	"fraud-detection-2026/internal/handlers"

	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()

	r.GET("/ready", handlers.Ready)

	r.Run(":9999")
}
