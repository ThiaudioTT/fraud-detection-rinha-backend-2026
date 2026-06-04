package handlers

import (
	"net/http"

	"fraud-detection-2026/internal/models"
	"fraud-detection-2026/internal/normalizer"
	"fraud-detection-2026/internal/service"

	"github.com/gin-gonic/gin"
)

// FraudHandler serves fraud-scoring requests. Its dependencies are wired once at
// startup and reused across requests, so the hot path allocates nothing beyond
// the request payload itself.
type FraudHandler struct {
	service *service.FraudService
}

func NewFraudHandler(s *service.FraudService) *FraudHandler {
	return &FraudHandler{service: s}
}

func (h *FraudHandler) FraudScore(c *gin.Context) {
	var payload models.FraudScoreRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid payload", "error": err.Error()})
		return
	}

	embedding := normalizer.NormalizePayloadTransaction(payload)
	result, err := h.service.ScoreTransaction(c.Request.Context(), embedding)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "failed to score fraud", "error": err.Error()})
		return
	}

	// result already carries json tags (approved, fraud_score).
	c.JSON(http.StatusOK, result)
}
