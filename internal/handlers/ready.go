package handlers

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ReadyHandler reports whether the instance can reach the database.
type ReadyHandler struct {
	pool *pgxpool.Pool
}

func NewReadyHandler(pool *pgxpool.Pool) *ReadyHandler {
	return &ReadyHandler{pool: pool}
}

func (h *ReadyHandler) Ready(c *gin.Context) {
	if err := h.pool.Ping(c.Request.Context()); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "not ready", "error": err.Error()})
		return
	}
	hostname, _ := os.Hostname() // For debugging which instance answered.
	c.JSON(http.StatusOK, gin.H{
		"message":  "ready",
		"hostname": hostname,
	})
}
