package handlers

import (
	"fraud-detection-2026/pkg/database"

	"github.com/gin-gonic/gin"
)

func Ready(c *gin.Context) {
	if err := database.Pool.Ping(c.Request.Context()); err != nil {
		c.JSON(503, gin.H{"message": "not ready", "error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"message": "ready",
	})
}
