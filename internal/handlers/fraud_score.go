package handlers

import (
	"net/http"

	"fraud-detection-2026/internal/models"
	"fraud-detection-2026/internal/normalizer"

	"github.com/gin-gonic/gin"
)

func FraudScore(c *gin.Context) {
	var payload models.FraudScoreRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid payload", "error": err.Error()})
		return
	}

	normalizedPayload := normalizer.NormalizePayloadTransaction(payload)

	c.JSON(http.StatusOK, gin.H{
		"approved":    false,
		"fraud_score": 1.0,
		"payload":     normalizedPayload, // TODO: remove this line after testing
	})
}
