package handlers

import (
	"net/http"

	"fraud-detection-2026/internal/models"
	"fraud-detection-2026/internal/normalizer"
	"fraud-detection-2026/internal/service"
	"fraud-detection-2026/pkg/database"

	"github.com/gin-gonic/gin"
)

func FraudScore(c *gin.Context) {
	var payload models.FraudScoreRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid payload", "error": err.Error()})
		return
	}

	normalizedPayload := normalizer.NormalizePayloadTransaction(payload)
	fraudService := service.NewFraudService(database.Pool) // FIXME: this should be injected, not created here
	approved, fraudScore, err := fraudService.ScoreTransaction(c.Request.Context(), normalizedPayload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "failed to score fraud", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"approved":    approved,
		"fraud_score": fraudScore,
	})
}
