package handlers

import (
	"net/http"

	"fraud-detection-2026/internal/index"

	"github.com/gin-gonic/gin"
)

// ReadyHandler reports whether the instance has the reference index loaded and
// ready to serve. The load balancer must not route traffic until this passes.
type ReadyHandler struct {
	idx *index.Index
}

func NewReadyHandler(idx *index.Index) *ReadyHandler {
	return &ReadyHandler{idx: idx}
}

func (h *ReadyHandler) Ready(c *gin.Context) {
	if h.idx == nil || h.idx.Count() == 0 {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "index not loaded"})
		return
	}
	c.Status(http.StatusOK)
}
