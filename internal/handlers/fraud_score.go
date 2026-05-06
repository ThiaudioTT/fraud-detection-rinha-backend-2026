package handlers

import "github.com/gin-gonic/gin"

func FraudScore(c *gin.Context) {
	c.JSON(200, gin.H{
		"approved":    false,
		"fraud_score": 1.0,
	})
}
